package kubernetes

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	digCmd := "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +search +showsearch +time=10 +tries=6"

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
	results, err := ParseDigResponse(cmdout)

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

// DoIntegrationTestUsingUpstreamServer executes a test case using the given upstream server.
func DoIntegrationTestWithUDPBufSize(tc test.Case, namespace string, bufsize string) (*dns.Msg, error) {
	digCmd := "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +ignore +bufsize=" + bufsize + " +search +showsearch +time=10 +tries=6"

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
	results, err := ParseDigResponse(cmdout)

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
				t.Errorf(err.Error())
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
func ParseDigResponse(r string) ([]*dns.Msg, error) {
	s := bufio.NewScanner(strings.NewReader(r))
	var msgs []*dns.Msg
	var err error

	for err == nil {
		m, err := parseDig(s)
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

// parseDig parses a single dig-like response and returns a dns.Msg
func parseDig(s *bufio.Scanner) (*dns.Msg, error) {
	m := new(dns.Msg)
	err := parseDigHeader(s, m)
	if err != nil {
		return nil, err
	}
	err = parseDigFlags(s, m)
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

func parseDigFlags(s *bufio.Scanner, m *dns.Msg) error {
	// Looking for the flags section of the header.
	flagsSection := ";; flags: "

	// Break out of the loop when the flags section is found.
	for {
		if strings.HasPrefix(s.Text(), flagsSection) {
			break
		}
		if !s.Scan() {
			return errors.New("flags section not found")
		}
	}

	// Copy the flags section of the header to a local variable.
	f := s.Text()

	// Extract the flags part of the header.
	flagsStart := strings.Index(f, "flags: ") + len("flags: ")
	flagsEnd := strings.Index(f, "; QUERY:")
	flagsStr := f[flagsStart:flagsEnd]

	// Split the flags string around each instance of white space characters.
	flags := strings.Fields(flagsStr)

	// Set the flags in the dns.Msg object.
	for _, flag := range flags {
		switch flag {
		case "qr":
			m.Response = true
		case "tc":
			m.Truncated = true
		case "rd":
			m.RecursionDesired = true
		case "ad":
			m.AuthenticatedData = true
		case "ra":
			m.RecursionAvailable = true
		case "aa":
			m.Authoritative = true
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
