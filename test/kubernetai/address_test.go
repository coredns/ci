package kubernetai

import (
	"fmt"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCases = []test.Case{
	{ // Query service served by last stanza
		Qname: "svc-1-a.test-1.svc.conglomeration.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.conglomeration.local.      30    IN      A       10.0.0.100"),
		},
	},
	{ // Query service served by first stanza
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      10    IN      A       10.0.0.100"),
		},
	},
	{ // Query service served by second stanza via fallthrough
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("kubernetes.default.svc.cluster.local.      20    IN      A       10.0.0.1"),
		},
	},
	{ // A PTR record in first stanza
		Qname: "100.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("100.0.0.10.in-addr.arpa. 10	IN	PTR	svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // A PTR record in second stanza
		Qname: "1.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("1.0.0.10.in-addr.arpa. 20	IN	PTR	kubernetes.default.svc.cluster.local."),
		},
	},
}

func TestKubernetai(t *testing.T) {

	corefile := `    .:53 {
        errors
        log
        kubernetai cluster.local 10.in-addr.arpa {
            namespaces test-1
            ttl 10
            fallthrough
        }
        kubernetai cluster.local 10.in-addr.arpa {
            ttl 20
        }
        kubernetai conglomeration.local {
            ttl 30
        }
    }
`

	err := kubernetes.LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCases
	namespace := "test-1"
	err = kubernetes.StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := kubernetes.DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			test.CNAMEOrder(t, res)
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", kubernetes.CorednsLogs())
			}
		})
	}
}
