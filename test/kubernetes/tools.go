package kubernetes

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin/test"
	ctest "github.com/coredns/coredns/test"

	"github.com/miekg/dns"
	// Load all managed plugins in github.com/coredns/coredns
	_ "github.com/coredns/coredns/core/plugin"
)

// DoIntegrationTest executes a test case
func DoIntegrationTest(tc test.Case, namespace string) (*dns.Msg, error) {
	var digCmd string
	var dp DigParser
	switch tc.Qtype {
	case dns.TypeAXFR:
		digCmd = "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +time=10 +tries=6"
		dp = ParseDigAXFR
	default:
		digCmd = "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +search +showsearch +time=10 +tries=6"
		dp = parseDig
	}

	// attach to client and execute query.
	var cmdout string
	var err error
	tries := 3
	for {
		cmdout, err = Kubectl("-n " + namespace + " exec " + clientName + " -- " + digCmd)
		if err == nil {
			break
		}
		tries = tries - 1
		if tries == 0 {
			return nil, errors.New("failed to execute query '" + digCmd + "' got error: '" + err.Error() + "'")
		}
		time.Sleep(500 * time.Millisecond)
	}
	results, err := ParseDigResponse(cmdout, dp)

	if err != nil {
		return nil, errors.New("failed to parse result: (" + err.Error() + ")" + cmdout)
	}
	if len(results) != 1 {
		resultStr := ""
		for i, r := range results {
			resultStr += fmt.Sprintf("\nResponse %v\n", i) + r.String()
		}
		return nil, errors.New("expected 1 query attempt, observed " + strconv.Itoa(len(results)) + resultStr)
	}
	return results[0], nil
}

// DoIntegrationTests executes test cases
func DoIntegrationTests(t *testing.T, testCases []test.Case, namespace string) {
	err := StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Error(err.Error())
			}
			test.CNAMEOrder(res)
			sort.Sort(test.RRSet(tc.Answer))
			sort.Sort(test.RRSet(tc.Ns))
			sort.Sort(test.RRSet(tc.Extra))
			if err := test.SortAndCheck(res, tc); err != nil {
				t.Error(err)
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}

// StartClientPod starts a dns client pod in the namespace
func StartClientPod(namespace string) error {
	_, err := Kubectl("-n " + namespace + " run " + clientName + " --image=infoblox/dnstools --restart=Never -- -c 'while [ 1 ]; do sleep 100; done'")
	if err != nil {
		// ignore error (pod already running)
		return nil
	}
	maxWait := 60 // 60 seconds
	for {
		o, _ := Kubectl("-n " + namespace + "  get pod " + clientName)
		if strings.Contains(o, "Running") {
			return nil
		}
		time.Sleep(time.Second)
		maxWait = maxWait - 1
		if maxWait == 0 {
			break
		}
	}
	return errors.New("timeout waiting for " + clientName + " to be ready")

}

// WaitForClientPodRecord waits for the client pod A record to be served by CoreDNS
func WaitForClientPodRecord(namespace string) error {
	maxWait := 120 // 120 seconds
	for {
		dashedip, err := Kubectl("-n " + namespace + " get pods -o wide " + clientName + " | grep " + clientName + " | awk '{print $6}' | tr . - | tr -d '\n'")
		if err == nil && dashedip != "" {
			digcmd := "dig -t a " + dashedip + "." + namespace + ".pod.cluster.local. +short | tr -d '\n'"
			digout, err := Kubectl("-n " + namespace + " exec " + clientName + " -- " + digcmd)
			if err == nil && digout != "" {
				return nil
			}
		}
		// wait and try again until timeout
		time.Sleep(time.Second)
		maxWait = maxWait - 1
		if maxWait == 0 {
			break
		}
	}
	return errors.New("timeout waiting for " + clientName + " A record.")
}

// UpstreamServer starts a local instance of coredns with the given zone file
func UpstreamServer(t *testing.T, zone, zoneFile string) (func(), *caddy.Instance, string) {
	upfile, rmFunc, err := test.TempFile(os.TempDir(), zoneFile)
	if err != nil {
		t.Fatalf("could not create file for CNAME upstream lookups: %s", err)
	}
	upstreamCorefile := `.:0 {
    file ` + upfile + ` ` + zone + `
    bind ` + locaIP().String() + `
}`
	server, udp, _, err := ctest.CoreDNSServerAndPorts(upstreamCorefile)
	if err != nil {
		t.Fatalf("could not get CoreDNS serving instance: %s", err)
	}

	return rmFunc, server, udp
}

// localIP returns the local system's first ipv4 non-loopback address
func locaIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		ip, _, _ := net.ParseCIDR(addr.String())
		ip = ip.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}
		return ip
	}
	return nil
}

// LoadCorefile calls loadCorefileAndZonefile without a zone file
func LoadCorefile(corefile string) error {
	return LoadCorefileAndZonefile(corefile, "", true)
}

// LoadCorefileAndZonefile constructs a configmap defining files for the corefile and zone,
// If restart is true, restarts the coredns pod to load the new configmap, and waits for the coredns pod to be ready.
func LoadCorefileAndZonefile(corefile, zonefile string, restart bool) error {

	// apply configmap yaml
	yamlString := configmap + "\n"
	yamlString += "  Corefile: |\n" + prepForConfigMap(corefile)
	yamlString += "  Zonefile: |\n" + prepForConfigMap(zonefile)

	file, rmFunc, err := test.TempFile(os.TempDir(), yamlString)
	if err != nil {
		return err
	}
	defer rmFunc()
	_, err = Kubectl("apply -f " + file)
	if err != nil {
		return err
	}

	if restart {
		// force coredns pod reload the config
		Kubectl("-n kube-system delete pods -l k8s-app=kube-dns")

		return WaitReady(30)
	}
	return nil
}

func LoadKubednsConfigmap(stubdata, upstreamdata string) error {

	//apply configmap yaml
	yamlString := KubednsConfigmap + "\n"
	yamlString += "  upstreamNameservers: |\n" + prepForConfigMap(upstreamdata)
	yamlString += "  stubDomains: |\n" + prepForConfigMap(stubdata)

	file, rmFunc, err := test.TempFile(os.TempDir(), yamlString)
	if err != nil {
		return err
	}
	defer rmFunc()

	_, err = Kubectl("apply -f " + file)
	if err != nil {
		return err
	}
	return nil
}

// WaitReady waits for 1 coredns to be ready or times out after maxWait seconds with an error
func WaitReady(maxWait int) error {
	return WaitNReady(maxWait, 1)
}

// WaitReady waits for n corednses to be ready or times out after maxWait seconds with an error
func WaitNReady(maxWait, n int) error {
	for {
		o, _ := Kubectl("-n kube-system get pods -l k8s-app=kube-dns -o jsonpath='{.items[*].status.containerStatuses[*].ready}'")
		if strings.Count(o, "true") == n {
			break
		}
		time.Sleep(time.Second)
		maxWait = maxWait - 1
		if maxWait == 0 {
			logs := CorednsLogs()
			return errors.New("timeout waiting for coredns to be ready. coredns log: " + logs)
		}
	}
	return nil
}

// CorednsLogs returns the current coredns log
func CorednsLogs() string {
	name, _ := Kubectl("-n kube-system get pods -l k8s-app=kube-dns | grep coredns | cut -f1 -d' ' | tr -d '\n'")
	logs, _ := Kubectl("-n kube-system logs " + name)
	return logs
}

// prepForConfigMap returns a config prepared for inclusion in a configmap definition
func prepForConfigMap(config string) string {
	var configOut string
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		// replace all tabs with spaces
		line = strings.Replace(line, "\t", "  ", -1)
		// indent line with 4 addtl spaces
		configOut += "    " + line + "\n"
	}
	return configOut
}

// CoreDNSPodIPs return the ips of all coredns pods
func CoreDNSPodIPs() ([]string, error) {
	lines, err := Kubectl("-n kube-system get pods -l k8s-app=kube-dns  -o wide | awk '{print $6}' | tail -n+2")
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, l := range strings.Split(lines, "\n") {
		p := net.ParseIP(l)
		if p == nil {
			continue
		}
		ips = append(ips, p.String())
	}
	return ips, nil
}

// HasResourceRestarted verifies if any of the specified containers in the kube-system namespace has restarted.
func HasResourceRestarted(label string) (bool, error) {
	// magic number
	wait := 5

	for {
		hasRestarted := false
		restartCount, err := Kubectl(fmt.Sprintf("-n kube-system get pods -l %s -ojsonpath='{.items[*].status.containerStatuses[0].restartCount}'", label))
		if err != nil {
			return false, err
		}
		individualCount := strings.Split(restartCount, " ")
		for _, count := range individualCount {
			if count != "0" {
				hasRestarted = true
			}
		}

		if hasRestarted {
			break
		}
		time.Sleep(time.Second)
		wait--
		if wait == 0 {
			return false, nil
		}
	}

	return true, nil
}

// FetchDockerContainerID fetches the docker container ID from the container name
func FetchDockerContainerID(containerName string) (string, error) {
	containerID, err := exec.Command("sh", "-c", fmt.Sprintf("docker ps -aqf \"name=%s\"", containerName)).CombinedOutput()
	if err != nil {
		return "", errors.New("error executing docker command to fetch container ID")
	}

	if containerID == nil {
		return "", errors.New("no containerID found")
	}

	return strings.TrimSpace(string(containerID)), nil
}

func ScrapeMetrics(t *testing.T) []byte {
	containerID, err := FetchDockerContainerID("kind-control-plane")
	if err != nil {
		t.Fatalf("docker container ID not found, err: %s", err)
	}

	ips, err := CoreDNSPodIPs()
	if err != nil {
		t.Errorf("could not get coredns pod ip: %v", err)
	}
	if len(ips) != 1 {
		t.Errorf("expected 1 pod ip, found: %v", len(ips))
	}

	ip := ips[0]
	cmd := fmt.Sprintf("docker exec -i %s /bin/sh -c \"curl -s http://%s:9153/metrics\"", containerID, ip)
	mf, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Errorf("error while trying to run command in docker container: %s %v", err, mf)
	}
	if len(mf) == 0 {
		t.Errorf("unable to scrape metrics from %v", ip)
	}
	return mf
}

// Kubectl executes the kubectl command with the given arguments
func Kubectl(args string) (result string, err error) {
	kctl := os.Getenv("KUBECTL")

	if kctl == "" {
		kctl = "kubectl"
	}

	cmdOut, err := exec.Command("sh", "-c", kctl+" "+args).CombinedOutput()
	if err != nil {
		return "", errors.New("got error '" + string(cmdOut) + "' for command " + kctl + " " + args)
	}
	return string(cmdOut), nil
}

// ParseDigResponse parses dig-like command output and returns a dns.Msg
func ParseDigResponse(r string, dp DigParser) ([]*dns.Msg, error) {
	s := bufio.NewScanner(strings.NewReader(r))
	var msgs []*dns.Msg
	var err error

	for err == nil {
		m, err := dp(s)
		if err != nil {
			break
		}
		if m == nil {
			return nil, errors.New("Unexpected nil message")
		}
		msgs = append(msgs, m)
	}

	if len(msgs) == 0 {
		return nil, err
	}
	return msgs, nil
}

// DigParser is a function that specialises in parsing different responses from running dig.
// The regular parseDig parser is acceptable for most tests, whilst the ParseDigAXFR handles this special case.
type DigParser func(s *bufio.Scanner) (*dns.Msg, error)

// parseDig parses a single dig-like response and returns a dns.Msg
func parseDig(s *bufio.Scanner) (*dns.Msg, error) {
	m := new(dns.Msg)
	err := parseDigHeader(s, m)
	if err != nil {
		return nil, err
	}
	err = parseDigQuestion(s, m)
	if err != nil {
		return nil, err
	}
	err = parseDigSections(s, m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ParseDigAXFR specifically parses AXFR responses which have a different format.
func ParseDigAXFR(s *bufio.Scanner) (*dns.Msg, error) {
	m := new(dns.Msg)
	for s.Scan() {
		if s.Text() == "" {
			continue
		}
		if strings.HasPrefix(s.Text(), ";") {
			continue
		}
		r, err := dns.NewRR(s.Text())
		if err != nil {
			fmt.Println("ParseDigAXFR RR record could not be parsed")
			return nil, err
		}
		m.Answer = append(m.Answer, r)
	}
	if len(m.Answer) == 0 {
		return nil, errors.New("no more records")
	}
	return m, nil
}

func parseDigHeader(s *bufio.Scanner, m *dns.Msg) error {
	headerSection := ";; ->>HEADER<<- "
	for {
		if strings.HasPrefix(s.Text(), headerSection) {
			break
		}
		if !s.Scan() {
			return errors.New("header section not found")
		}
	}
	l := s.Text()
	strings.Replace(l, headerSection, "", 1)
	nvps := strings.Split(l, ", ")
	for _, nvp := range nvps {
		nva := strings.Split(nvp, ": ")
		if nva[0] == "opcode" {
			m.Opcode = invertIntMap(dns.OpcodeToString)[nva[1]]
		}
		if nva[0] == "status" {
			m.Rcode = invertIntMap(dns.RcodeToString)[nva[1]]
		}
		if nva[0] == "id" {
			i, err := strconv.Atoi(nva[1])
			if err != nil {
				return err
			}
			m.MsgHdr.Id = uint16(i)
		}
	}
	return nil
}

func parseDigQuestion(s *bufio.Scanner, m *dns.Msg) error {
	for {
		if strings.HasPrefix(s.Text(), ";; QUESTION SECTION:") {
			break
		}
		if !s.Scan() {
			return errors.New("question section not found")
		}
	}
	s.Scan()
	l := s.Text()
	l = strings.TrimLeft(l, ";")
	fields := strings.Fields(l)
	m.SetQuestion(fields[0], invertUint16Map(dns.TypeToString)[fields[2]])
	return nil
}

func parseDigSections(s *bufio.Scanner, m *dns.Msg) error {
	var section string
	for s.Scan() {
		if strings.HasSuffix(s.Text(), " SECTION:") {
			section = strings.Fields(s.Text())[1]
			continue
		}
		if s.Text() == "" {
			continue
		}
		if strings.HasPrefix(s.Text(), ";;") {
			break
		}
		r, err := dns.NewRR(s.Text())
		if err != nil {
			return err
		}
		if section == "ANSWER" {
			m.Answer = append(m.Answer, r)
		}
		if section == "AUTHORITY" {
			m.Ns = append(m.Ns, r)
		}
		if section == "ADDITIONAL" {
			m.Extra = append(m.Extra, r)
		}
	}
	return nil
}

func invertIntMap(m map[int]string) map[string]int {
	n := make(map[string]int)
	for k, v := range m {
		n[v] = k
	}
	return n
}

func invertUint16Map(m map[uint16]string) map[string]uint16 {
	n := make(map[string]uint16)
	for k, v := range m {
		n[v] = k
	}
	return n
}

// configmap is the header used for defining the coredns configmap
const (
	configmap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:`

	// KubednsConfigmap is the header used for defining the kube-dns configmap
	KubednsConfigmap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-dns
  namespace: kube-system
data:
`

	// ExampleNet is an example upstream zone file
	ExampleNet = `; example.net. test file for cname tests
example.net.          IN      SOA     ns.example.net. admin.example.net. 2015082541 7200 3600 1209600 3600
example.net. IN A 13.14.15.16
`
	clientName     = "coredns-test-client"
	CoreDNSLabel   = "k8s-app=kube-dns"
	APIServerLabel = "component=kube-apiserver"
)

// ValidateAXFR compares the dns records returned against a set of expected records.
// It ensures that the axfr response begins and ends with an SOA record.
// It will only test the first 3 tuples of each A record.
func ValidateAXFR(xfr []dns.RR, expected []dns.RR) []error {
	var failures []error
	if xfr[0].Header().Rrtype != dns.TypeSOA {
		failures = append(failures, errors.New("Invalid transfer response, does not start with SOA record"))
	}
	if xfr[len(xfr)-1].Header().Rrtype != dns.TypeSOA {
		failures = append(failures, errors.New("Invalid transfer response, does not end with SOA record"))
	}

	// make a map of xfr responses to search...
	xfrMap := make(map[int]dns.RR, len(xfr))
	for i := range xfr {
		xfrMap[i] = xfr[i]
	}

	// for each expected entry find a result response which matches.
	for i := range expected {
		matched := false
		for key, resultRR := range xfrMap {
			hmatch, err := matchHeader(expected[i].Header(), resultRR.Header())
			if err != nil {
				failures = append(failures, err)
				break
			}
			if !hmatch {
				continue
			}

			// headers match
			// special matchers and default full match
			switch expected[i].Header().Rrtype {
			case dns.TypeSOA, dns.TypeA:
				matched = true
				break
			case dns.TypeSRV:
				srvMatch, err := matchSRVResponse(expected[i].(*dns.SRV), resultRR.(*dns.SRV))
				if err != nil {
					failures = append(failures, err)
					break
				}
				if srvMatch {
					matched = true
					break
				}
			default:
				if dns.IsDuplicate(expected[i], resultRR) {
					matched = true
				}
			}

			if matched {
				delete(xfrMap, key)
				break
			}
		}
		if !matched {
			failures = append(failures, fmt.Errorf("the expected AXFR record not found in results:\n%s\n", expected[i]))
		}
	}

	// catch unexpected extra records
	if len(xfrMap) > 0 {
		for _, r := range xfrMap {
			failures = append(failures, fmt.Errorf("additional axfr record not expected: %s", r.String()))
		}
	}
	return failures
}

// matchHeader will return true when two headers are exactly equal or the expected and resultant header
// both contain a dashed ip address and the domain matches.
func matchHeader(expected, result *dns.RR_Header) (bool, error) {
	if expected.Rrtype != result.Rrtype {
		return false, nil
	}
	if expected.Class != result.Class {
		return false, nil
	}
	if expected.Rrtype != result.Rrtype {
		return false, nil
	}
	expectedNameReg, err := zoneToRelaxedRegex(expected.Name)
	if err != nil {
		return false, fmt.Errorf("failed to covert dns name %s to regex: %v", expected.Name, err)
	}
	if !expectedNameReg.MatchString(result.Name) {
		return false, nil
	}
	return true, nil
}

// validateSRVResponse matches an SRV response record
func matchSRVResponse(expectedSRV, resultSRV *dns.SRV) (bool, error) {
	// test other SRV record attributes...
	if expectedSRV.Port != resultSRV.Port {
		return false, nil
	}
	if expectedSRV.Priority != resultSRV.Priority {
		return false, nil
	}
	if expectedSRV.Weight != resultSRV.Weight {
		return false, nil
	}

	expectedTargetReg, err := zoneToRelaxedRegex(expectedSRV.Target)
	if err != nil {
		return false, fmt.Errorf("failed to covert srv target %s to regex: %v", expectedSRV.Target, err)
	}
	if !expectedTargetReg.MatchString(resultSRV.Target) {
		return false, nil
	}
	return true, nil
}

var ipPartMatcher = regexp.MustCompile(`^\d+-\d+-\d+-\d+\.`)

// zoneToRelaxedRegex creates a regular expression from a domain name, replacing ipv4 dashed addresses with a
// more generalised matcher that will match any address.
func zoneToRelaxedRegex(source string) (*regexp.Regexp, error) {
	if !ipPartMatcher.MatchString(source) {
		return regexp.Compile(`^` + source + `$`)
	}
	return regexp.Compile(ipPartMatcher.ReplaceAllString(source, `^\d+-\d+-\d+-\d+\.`) + `$`)
}
