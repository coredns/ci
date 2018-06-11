package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var upstreamCases = []test.Case{
	{ // An externalName service should result in a CNAME plus the A record
		Qname: "ext-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("example.net.                     303    IN      A       13.14.15.16"),
			test.CNAME("ext-svc.test-1.svc.cluster.local.  303    IN      CNAME   example.net."),
		},
	},
}

func TestUpstreamToSelf(t *testing.T) {
	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local {
            upstream
        }
        file /etc/coredns/Zonefile example.net
    }
`

	err := LoadCorefileAndZonefile(corefile, ExampleNet)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := upstreamCases
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

func TestUpstreamToOther(t *testing.T) {
	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local {
            upstream ` + udp + `
        }
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := upstreamCases
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

func TestUpstreamLoopBreak(t *testing.T) {
	var testCases = []test.Case{
		{ // A CNAME loop should break and return the records in the loop
			Qname: "upriver.test-2.svc.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.CNAME("up.river.local                     303  IN  CNAME  upriver.test-2.svc.cluster.local."),
				test.CNAME("upriver.test-2.svc.cluster.local.  303  IN  CNAME  up.river.local."),
			},
		},
	}
	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local {
            upstream
        }
        template ANY ANY up.river.local {
          answer "up.river.local 5 IN CNAME upriver.test-2.svc.cluster.local."
          upstream
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
