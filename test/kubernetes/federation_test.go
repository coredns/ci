package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestFederation(t *testing.T) {
	var testCases = []test.Case{
		{ // A query of a federated service should return a CNAME and A record
			Qname: "fedsvc.default.testfed.svc.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.CNAME("fedsvc.default.testfed.svc.cluster.local. 303  IN  CNAME  fedsvc.default.testfed.svc.fdzone.fdregion.example.com."),
				test.A("fedsvc.default.testfed.svc.fdzone.fdregion.example.com. 303 IN A 1.2.3.4"),
			},
		},
	}
	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local
        federation cluster.local {
          upstream
          testfed example.com
        }
        template ANY ANY example.com {
          answer "fedsvc.default.testfed.svc.fdzone.fdregion.example.com. 5 IN A 1.2.3.4"
        }
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
			// Dedup any duplicates that the template plugin may create
			res.Answer = dns.Dedup(res.Answer, nil)
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
