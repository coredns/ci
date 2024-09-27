package kubernetai

import (
	"bufio"
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

// load answers turns a text based dig response into a set of answers
func loadAXFRAnswers(t *testing.T, results string) []dns.RR {
	s := bufio.NewScanner(strings.NewReader(results))
	answers, err := kubernetes.ParseDigAXFR(s)
	if err != nil {
		t.Fatalf("failed to parse expected AXFR results: %v", err)
		return []dns.RR{}
	}
	return answers.Answer
}

func TestAXFR(t *testing.T) {
	testCases := map[string]struct {
		Config string
		dig    test.Case
	}{
		"matches stanza 1": {
			Config: `    .:53 {
        health
        ready
        errors
        log
        kubernetai test-4.svc.cluster.local 10.in-addr.arpa {
            namespaces test-4
            fallthrough
        }
        kubernetai test-5.svc.cluster.local 10.in-addr.arpa {
			namespaces test-5
			pods verified
			endpoint_pod_names
		}
		transfer {
			to *
		}
    }
`,
			dig: test.Case{
				Qname: "test-4.svc.cluster.local.", Qtype: dns.TypeAXFR,
				Rcode: dns.RcodeSuccess,
				Answer: loadAXFRAnswers(t, `
test-4.svc.cluster.local. 5	IN	SOA	ns.dns.test-4.svc.cluster.local. hostmaster.test-4.svc.cluster.local. 1726484129 7200 1800 86400 5
ext-svc.test-4.svc.test-4.svc.cluster.local. 5 IN CNAME	example.net.
headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN A 172.17.0.252
svc-d.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN A 172.17.0.252
_c-port._udp.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN SRV	0 50 1234 svc-d.headless-svc.test-4.svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN A 172.17.0.253
172-17-0-253.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN A 172.17.0.253
_c-port._udp.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN SRV	0 50 1234svc-1-a.headless-svc.test-4.svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
headless-svc-3.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
_c-port._udp.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN SRV	0 50 1234 headless-svc-3.headless-svc.test-4.svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
1234-abcd--4.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
_c-port._udp.headless-svc.test-4.svc.test-4.svc.cluster.local. 5 IN SRV	0 50 1234 1234-abcd--4.headless-svc.test-4.svc.test-4.svc.cluster.local.
svc-1-a.test-4.svc.test-4.svc.cluster.local. 5 IN A 10.96.0.200
svc-1-a.test-4.svc.test-4.svc.cluster.local. 5 IN SRV 0 100 80 svc-1-a.test-4.svc.test-4.svc.cluster.local.
_http._tcp.svc-1-a.test-4.svc.test-4.svc.cluster.local.	5 IN SRV 0 100 80 svc-1-a.test-4.svc.test-4.svc.cluster.local.
svc-1-a.test-4.svc.test-4.svc.cluster.local. 5 IN SRV 0 100 443 svc-1-a.test-4.svc.test-4.svc.cluster.local.
_https._tcp.svc-1-a.test-4.svc.test-4.svc.cluster.local. 5 IN SRV 0 100 443 svc-1-a.test-4.svc.test-4.svc.cluster.local.
svc-1-b.test-4.svc.test-4.svc.cluster.local. 5 IN A 10.96.0.210
svc-1-b.test-4.svc.test-4.svc.cluster.local. 5 IN SRV 0 100 80 svc-1-b.test-4.svc.test-4.svc.cluster.local.
_http._tcp.svc-1-b.test-4.svc.test-4.svc.cluster.local.	5 IN SRV 0 100 80 svc-1-b.test-4.svc.test-4.svc.cluster.local.
svc-c.test-4.svc.test-4.svc.cluster.local. 5 IN	A 10.96.0.215
svc-c.test-4.svc.test-4.svc.cluster.local. 5 IN	SRV 0 100 1234 svc-c.test-4.svc.test-4.svc.cluster.local.
_c-port._udp.svc-c.test-4.svc.test-4.svc.cluster.local.	5 IN SRV 0 100 1234 svc-c.test-4.svc.test-4.svc.cluster.local.
test-4.svc.cluster.local. 5	IN	SOA	ns.dns.test-4.svc.cluster.local. hostmaster.test-4.svc.cluster.local. 1726484129 7200 1800 86400 5
`),
			},
		},
		"matches stanza 2": {
			Config: `    .:53 {
        health
        ready
        errors
        log
        kubernetai test-4.svc.cluster.local 10.in-addr.arpa {
            namespaces test-4
            fallthrough
        }
        kubernetai test-5.svc.cluster.local 10.in-addr.arpa {
			namespaces test-5
			pods verified
			endpoint_pod_names
		}
		transfer {
			to *
		}
    }
`,
			dig: test.Case{
				Qname: "test-5.svc.cluster.local.", Qtype: dns.TypeAXFR,
				Rcode: dns.RcodeSuccess,
				Answer: loadAXFRAnswers(t, `
test-5.svc.cluster.local. 5	IN	SOA	ns.dns.test-5.svc.cluster.local. hostmaster.test-5.svc.cluster.local. 1726484386 7200 1800 86400 5
headless-1.test-5.svc.test-5.svc.cluster.local.	5 IN A 172.17.0.173
test-name.headless-1.test-5.svc.test-5.svc.cluster.local. 5 IN A 172.17.0.173
_http._tcp.headless-1.test-5.svc.test-5.svc.cluster.local. 5 IN	SRV 0 100 80 test-name.headless-1.test-5.svc.test-5.svc.cluster.local.
headless-2.test-5.svc.test-5.svc.cluster.local.	5 IN A 172.17.0.182
172-17-0-182.headless-2.test-5.svc.test-5.svc.cluster.local. 5 IN A 172.17.0.182
_http._tcp.headless-2.test-5.svc.test-5.svc.cluster.local. 5 IN	SRV 0 100 80 172-17-0-182.headless-2.test-5.svc.test-5.svc.cluster.local.
test-5.svc.cluster.local. 5	IN	SOA	ns.dns.test-5.svc.cluster.local. hostmaster.test-5.svc.cluster.local. 1726484386 7200 1800 86400 5
`),
			},
		},
		"matches first stanza": {
			Config: `    .:53 {
        health
        ready
        errors
        log
        kubernetai cluster.local 10.in-addr.arpa {
            namespaces test-4
            fallthrough
        }
        kubernetai cluster.local 10.in-addr.arpa {
			namespaces test-5
			pods verified
			endpoint_pod_names
		}
		transfer {
			to *
		}
    }
`,
			dig: test.Case{
				Qname: "cluster.local.", Qtype: dns.TypeAXFR,
				Rcode: dns.RcodeSuccess,
				Answer: loadAXFRAnswers(t, `
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726499016 7200 1800 86400 5
ext-svc.test-4.svc.cluster.local. 5 IN	CNAME	example.net.
headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
headless-svc-3.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::3
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 headless-svc-3.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
1234-abcd--4.headless-svc.test-4.svc.cluster.local. 5 IN AAAA 1234:abcd::4
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 1234-abcd--4.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.249
172-17-0-249.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.249
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 172-17-0-249.headless-svc.test-4.svc.cluster.local.
headless-svc.test-4.svc.cluster.local. 5 IN A	172.17.0.250
172-17-0-250.headless-svc.test-4.svc.cluster.local. 5 IN A 172.17.0.250
_c-port._udp.headless-svc.test-4.svc.cluster.local. 5 IN SRV 0 50 1234 172-17-0-250.headless-svc.test-4.svc.cluster.local.
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
cluster.local.		5	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1726499016 7200 1800 86400 5`),
			},
		},
	}

	for description, tc := range testCases {
		t.Run(fmt.Sprintf("%s/%s %s", description, tc.dig.Qname, dns.TypeToString[tc.dig.Qtype]), func(t *testing.T) {
			err := kubernetes.LoadCorefile(tc.Config)
			if err != nil {
				t.Fatalf("Could not load corefile: %s", err)
			}
			namespace := "test-1"
			err = kubernetes.StartClientPod(namespace)
			if err != nil {
				t.Fatalf("failed to start client pod: %s", err)
			}

			res, err := kubernetes.DoIntegrationTest(tc.dig, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			if res != nil {
				failures := kubernetes.ValidateAXFR(res.Answer, tc.dig.Answer)
				for _, flr := range failures {
					t.Error(flr)
				}
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", kubernetes.CorednsLogs())
			}
		})
	}
}
