package kubernetes

import (
	"bufio"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// load answers turns a text based dig response into a set of answers
func loadAXFRAnswers(t *testing.T, results string) []dns.RR {
	s := bufio.NewScanner(strings.NewReader(results))
	answers, err := ParseDigAXFR(s)
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
headless-svc-3.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 headless-svc-3.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
1234-abcd--4.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 1234-abcd--4.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.252
svc-d.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.252
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 svc-d.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.253
172-17-0-253.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.253
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 svc-1-a.headless-svc.test-4.svc.cluster.local.
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
				failures := ValidateAXFR(res.Answer, tc.Answer)
				for _, flr := range failures {
					t.Error(flr)
				}
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
				failures := ValidateAXFR(res.Answer, tc.Answer)
				for _, flr := range failures {
					t.Error(flr)
				}
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
