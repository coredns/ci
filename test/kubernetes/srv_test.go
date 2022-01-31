package kubernetes

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var dnsTestCasesSRV = []test.Case{
	{
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.96.0.100"),
		},
	},
}

func TestKubernetesSRV(t *testing.T) {

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        health
        ready
	    errors
	    log
        kubernetes cluster.local {
            namespaces test-1
		}
		forward . ` + udp + `
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCasesSRV
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
