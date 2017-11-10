package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesPTR = []test.Case{
	{ // An PTR record query for an existing service should return a record
		Qname: "100.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("100.0.0.10.in-addr.arpa. 303	IN	PTR	svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // An PTR record query for an existing endpoint should return a record
		Qname: "253.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("253.0.17.172.in-addr.arpa. 303	IN	PTR	172-17-0-253.svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // An PTR record query for an existing service in an UNEXPOSED namespace should return NODATA
		Qname: "120.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("0.0.10.in-addr.arpa.	303	IN	SOA	ns.dns.0.0.10.in-addr.arpa. hostmaster.0.0.10.in-addr.arpa. 1510339777 7200 1800 86400 30"),
		},
	},
	{ // An PTR record query for an existing endpoint in an UNEXPOSED namespace should return NODATA
		Qname: "252.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("0.17.172.in-addr.arpa.	303	IN	SOA	ns.dns.0.17.172.in-addr.arpa. hostmaster.0.17.172.in-addr.arpa. 1510339711 7200 1800 86400 30"),
		},
	},
	{ // An PTR record query for an ip address that is not a service or endpoint should return NODATA
		Qname: "200.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Ns: []dns.RR{
			test.SOA("0.17.172.in-addr.arpa.	303	IN	SOA	ns.dns.0.17.172.in-addr.arpa. hostmaster.0.17.172.in-addr.arpa. 1510339711 7200 1800 86400 30"),
		},
	},
}

func TestKubernetesPTR(t *testing.T) {
	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local 10.0.0.0/24 172.17.0.0/24 {
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
			test.CNAMEOrder(t, res)
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
