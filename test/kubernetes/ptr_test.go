package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesPTR = []test.Case{
	{ // A PTR record query for an existing service should return a record
		Qname: "100.0.96.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("100.0.96.10.in-addr.arpa. 303	IN	PTR	svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // A PTR record query for an existing endpoint should return a record
		Qname: "253.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("253.0.17.172.in-addr.arpa. 303	IN	PTR	172-17-0-253.svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // A PTR record query for an existing ipv6 endpoint should return a record
		Qname: "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.d.c.b.a.4.3.2.1.ip6.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.d.c.b.a.4.3.2.1.ip6.arpa. 303 IN PTR 1234-abcd--1.headless-svc.test-1.svc.cluster.local."),
		},
	},
	{ // A PTR record query for an existing service in an UNEXPOSED namespace should return NXDOMAIN
		Qname: "120.0.96.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("0.96.10.in-addr.arpa.	303	IN	SOA	ns.dns.0.96.10.in-addr.arpa. hostmaster.0.96.10.in-addr.arpa. 1510339777 7200 1800 86400 30"),
		},
	},
	{ // A PTR record query for an existing endpoint in an UNEXPOSED namespace should return NXDOMAIN
		Qname: "252.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("0.17.172.in-addr.arpa.	303	IN	SOA	ns.dns.0.17.172.in-addr.arpa. hostmaster.0.17.172.in-addr.arpa. 1510339711 7200 1800 86400 30"),
		},
	},
	{ // A PTR record query for an ip address that is not a service or endpoint should return NXDOMAIN
		Qname: "200.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("0.17.172.in-addr.arpa.	303	IN	SOA	ns.dns.0.17.172.in-addr.arpa. hostmaster.0.17.172.in-addr.arpa. 1510339711 7200 1800 86400 30"),
		},
	},
}

func TestKubernetesPTR(t *testing.T) {
	corefile := `    .:53 {
        ready
        errors
        log
        kubernetes cluster.local 10.96.0.0/24 172.17.0.0/24 1234:abcd::0/64 {
            namespaces test-1
        }
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCasesPTR
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
			test.CNAMEOrder(res)
			if err := test.SortAndCheck(res, tc); err != nil {
				t.Error(err)
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
