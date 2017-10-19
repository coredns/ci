package kubernetes

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	intTest "github.com/coredns/coredns/test"

	"errors"
	"fmt"
	"github.com/mholt/caddy"
	"github.com/miekg/dns"
	"time"
)

func DoIntegrationTest(tc test.Case, namespace string) (*dns.Msg, error) {
	digCmd := "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +search +showsearch +time=10 +tries=6"

	// attach to client and execute query.
	var cmdout string
	var err error
	tries := 3
	for {
		cmdout, err = kubectl("-n " + namespace + " exec " + clientName + " -- " + digCmd)
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
		return nil, errors.New("expected 1 query attempt, observed " + strconv.Itoa(len(results)))
	}
	return results[0], nil
}

// doIntegrationTests executes test cases
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
			test.CNAMEOrder(t, res)
			sort.Sort(test.RRSet(tc.Answer))
			sort.Sort(test.RRSet(tc.Ns))
			sort.Sort(test.RRSet(tc.Extra))
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", corednsLogs())
			}
		})
	}
}

func StartClientPod(namespace string) error {
	_, err := kubectl("-n " + namespace + " run " + clientName + " --image=infoblox/dnstools --restart=Never -- -c 'while [ 1 ]; do sleep 100; done'")
	if err != nil {
		// ignore error (pod already running)
		return nil
	}
	maxWait := 60 // 60 seconds
	for {
		o, _ := kubectl("-n " + namespace + "  get pod " + clientName)
		if strings.Contains(o, "Running") {
			return nil
		}
		time.Sleep(time.Second)
		maxWait = maxWait - 1
		if maxWait == 0 {
			break
		}
	}
	return errors.New("timeout waiting for " + clientName + " to be ready.")

}

// UpstreamServer starts a local instance of coredns with the given zone file
func UpstreamServer(t *testing.T, zone, zoneFile string) (func(), *caddy.Instance, string) {
	upfile, rmFunc, err := intTest.TempFile(os.TempDir(), zoneFile)
	if err != nil {
		t.Fatalf("could not create file for CNAME upstream lookups: %s", err)
	}
	upstreamCorefile := `.:0 {
    file ` + upfile + ` ` + zone + `
    bind ` + locaIP().String() + `
}`
	server, udp, _, err := intTest.CoreDNSServerAndPorts(upstreamCorefile)
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
	return LoadCorefileAndZonefile(corefile, "")
}

// LoadCorefileAndZonefile constructs and configmap defining files for the corefile and zone,
// forces the coredns pod to load the new configmap, and waits for the coredns pod to be ready.
func LoadCorefileAndZonefile(corefile, zonefile string) error {

	// apply configmap yaml
	yamlString := configmap + "\n"
	yamlString += "  Corefile: |\n" + prepForConfigMap(corefile)
	yamlString += "  Zonefile: |\n" + prepForConfigMap(zonefile)

	file, rmFunc, err := intTest.TempFile(os.TempDir(), yamlString)
	if err != nil {
		return err
	}
	defer rmFunc()
	_, err = kubectl("apply -f " + file)
	if err != nil {
		return err
	}

	// force coredns pod reload the config
	kubectl("-n kube-system delete pods -l k8s-app=coredns")

	// wait for coredns to be ready before continuing
	maxWait := 30 // 15 seconds (each failed check sleeps 0.5 seconds)
	running := 0
	for {
		o, _ := kubectl("-n kube-system get pods -l k8s-app=coredns")
		if strings.Contains(o, "Running") {
			running += 1
		}
		if running >= 8 {
			// give coredns a chance to read its config before declaring victory
			break
		}
		time.Sleep(500 * time.Millisecond)
		maxWait = maxWait - 1
		if maxWait == 0 {
			//println(o)
			logs := corednsLogs()
			return errors.New("timeout waiting for coredns to be ready. coredns log: " + logs)
		}
	}
	return nil
}

func corednsLogs() string {
	name, _ := kubectl("-n kube-system get pods -l k8s-app=coredns | grep Running | cut -f1 -d' ' | tr -d '\n'")
	logs, _ := kubectl("-n kube-system logs " + name)
	return (logs)
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

// kubectl executes the kubectl command with the given arguments
func kubectl(args string) (result string, err error) {
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

	// ExampleNet is an example upstream zone file
	ExampleNet = `; example.net. test file for cname tests
example.net.          IN      SOA     ns.example.net. admin.example.net. 2015082541 7200 3600 1209600 3600
example.net. IN A 13.14.15.16
`
	clientName = "coredns-test-client"
)
