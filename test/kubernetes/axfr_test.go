package kubernetes

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// load answers turns a text based dig response into a set of answers
func loadAXFRAnswers(t *testing.T, results string) []dns.RR {
	s := bufio.NewScanner(strings.NewReader(results))
	answers, err := parseDigAXFR(s)
	if err != nil {
		t.Fatalf("failed to parse expected AXFR results: %v", err)
		return []dns.RR{}
	}
	return answers.Answer
}

func TestAXFR(t *testing.T) {
	testCases := []test.Case{
		{ // An A record query for an existing service should return a record
			Qname: "cluster.local.", Qtype: dns.TypeAXFR,
			Rcode: dns.RcodeSuccess,
			Answer: loadAXFRAnswers(t, `
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726438867 7200 1800 86400 5
cluster.local.		5	IN	NS	kube-dns.kube-system.svc.cluster.local.
kube-dns.kube-system.svc.cluster.local.	0 IN A	10.96.0.10
ext-svc.test-4.svc.cluster.local. 5 IN	CNAME	example.net.
headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
1234-abcd--3.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 1234-abcd--3.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
1234-abcd--4.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 1234-abcd--4.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.252
172-17-0-252.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.252
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 172-17-0-252.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.253
172-17-0-253.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.253
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 172-17-0-253.headless-svc.test-4.svc.cluster.local.
svc-1-a.test-4.svc.cluster.local. 5 IN	A	10.96.0.200
svc-1-a.test-4.svc.cluster.local. 5 IN	SRV	0 100 80 svc-1-a.test-4.svc.cluster.local.
_http._tcp.svc-1-a.test-4.svc.cluster.local. 5 IN SRV 0 100 80 svc-1-a.test-4.svc.cluster.local.
svc-1-a.test-4.svc.cluster.local. 5 IN	SRV	0 100 443 svc-1-a.test-4.svc.cluster.local.
_https._tcp.svc-1-a.test-4.svc.cluster.local. 5	IN SRV 0 100 443 svc-1-a.test-4.svc.cluster.local.
svc-1-b.test-4.svc.cluster.local. 5 IN	A	10.96.0.210
svc-1-b.test-4.svc.cluster.local. 5 IN	SRV	0 100 80 svc-1-b.test-4.svc.cluster.local.
_http._tcp.svc-1-b.test-4.svc.cluster.local. 5 IN SRV 0 100 80 svc-1-b.test-4.svc.cluster.local.
svc-c.test-4.svc.cluster.local.	5 IN	A	10.96.0.215
svc-c.test-4.svc.cluster.local.	5 IN	SRV	0 100 1234 svc-c.test-4.svc.cluster.local.
_c-port._udp.svc-c.test-4.svc.cluster.local. 5 IN SRV 0 100 1234 svc-c.test-4.svc.cluster.local.
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726438867 7200 1800 86400 5
`),
		},
	}

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        health
        ready
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
			namespaces test-4
		}
		transfer {
			to *
		}
		forward . ` + udp + `
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	namespace := "test-1"
	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			if res != nil {
				validateAXFR(t, res.Answer, tc.Answer)
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}

func TestAXFRPods(t *testing.T) {
	testCases := []test.Case{
		{ // An A record query for an existing service should return a record
			Qname: "cluster.local.", Qtype: dns.TypeAXFR,
			Rcode: dns.RcodeSuccess,
			Answer: loadAXFRAnswers(t, `
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726233714 7200 1800 86400 5
cluster.local.		5	IN	NS	kube-dns.kube-system.svc.cluster.local.
kube-dns.kube-system.svc.cluster.local.	0 IN A	10.96.0.10
headless-1.test-5.svc.cluster.local. 5 IN A	172.17.0.166
test-name.headless-1.test-5.svc.cluster.local. 5 IN A 172.17.0.166
_http._tcp.headless-1.test-5.svc.cluster.local.	5 IN SRV 0 100 80 test-name.headless-1.test-5.svc.cluster.local.
headless-2.test-5.svc.cluster.local. 5 IN A	172.17.0.167
172-17-0-167.headless-2.test-5.svc.cluster.local. 5 IN A 172.17.0.167
_http._tcp.headless-2.test-5.svc.cluster.local.	5 IN SRV 0 100 80 0-0-0-9.headless-2.test-5.svc.cluster.local.
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726233714 7200 1800 86400 5
`),
		},
	}

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        health
        ready
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
			namespaces test-5
			pods verified
			endpoint_pod_names
		}
		transfer {
			to *
		}
		forward . ` + udp + `
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	namespace := "test-1"
	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			if res != nil {
				validateAXFR(t, res.Answer, tc.Answer)
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}

// validateAXFR compares the dns records returned against a set of expected records.
// It ensures that the axfr response begins and ends with an SOA record.
// It will only test the first 3 tuples of each A record.
func validateAXFR(t *testing.T, xfr []dns.RR, expected []dns.RR) {
	if xfr[0].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid transfer response, does not start with SOA record")
	}
	if xfr[len(xfr)-1].Header().Rrtype != dns.TypeSOA {
		t.Error("Invalid transfer response, does not end with SOA record")
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
			if !matchHeader(t, expected[i].Header(), resultRR.Header()) {
				continue
			}

			// headers match
			// special matchers and default full match
			switch expected[i].Header().Rrtype {
			case dns.TypeSOA, dns.TypeA:
				matched = true
				break
			case dns.TypeSRV:
				if matchSRVResponse(t, expected[i].(*dns.SRV), resultRR.(*dns.SRV)) {
					matched = true
				}
				break
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
			t.Errorf("this AXFR record does not match any results:\n%s\n", expected[i])
		}
	}

	if len(xfr) > len(expected) {
		t.Errorf("Invalid number of responses, want %d, got %d", len(expected), len(xfr))
	}
}

// matchHeader will return true when two headers are exactly equal or the expected and resultant header
// both contain a dashed ip address and the domain matches.
func matchHeader(t *testing.T, expected, result *dns.RR_Header) bool {
	if expected.Rrtype != result.Rrtype {
		return false
	}
	if expected.Class != result.Class {
		return false
	}
	if expected.Rrtype != result.Rrtype {
		return false
	}
	expectedNameReg, err := zoneToRelaxedRegex(expected.Name)
	if err != nil {
		t.Fatalf("failed to covert dns name %s to regex: %v", expected.Name, err)
	}
	if !expectedNameReg.MatchString(result.Name) {
		return false
	}
	return true
}

// validateSRVResponse matches an SRV response record
func matchSRVResponse(t *testing.T, expectedSRV, resultSRV *dns.SRV) bool {
	expectedTargetReg, err := zoneToRelaxedRegex(expectedSRV.Target)
	if err != nil {
		t.Fatalf("failed to covert srv target %s to regex: %v", expectedSRV.Target, err)
	}
	if !expectedTargetReg.MatchString(resultSRV.Target) {
		return false
	}

	// test other SRV record attributes...
	if expectedSRV.Port != resultSRV.Port {
		return false
	}
	if expectedSRV.Priority != resultSRV.Priority {
		return false
	}
	if expectedSRV.Weight != resultSRV.Weight {
		return false
	}
	return true
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
