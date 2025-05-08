package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesPodsInsecure = []test.Case{
	{
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("10-20-0-101.test-1.pod.cluster.local. 303 IN A    10.20.0.101"),
		},
	},
	{
		Qname: "10-20-0-101.test-X.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502307903 7200 1800 86400 60"),
		},
	},
}

func TestKubernetesPodsInsecure(t *testing.T) {
	corefile := `    .:53 {
      health
      ready
	  errors
	  log
      kubernetes cluster.local {
                namespaces test-1
                pods insecure
      }
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCasesPodsInsecure
	namespace := "test-1"
	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Error(err.Error())
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

var dnsTestCasesPodsVerified = []test.Case{
	{
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502308197 7200 1800 86400 60"),
		},
	},
	{
		Qname: "10-20-0-101.test-X.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502307960 7200 1800 86400 60"),
		},
	},
}

func TestKubernetesPodsVerified(t *testing.T) {
	corefile := `    .:53 {
      health
      ready
	  errors
	  log
      kubernetes cluster.local {
                namespaces test-1
                pods verified
      }
    }
`
	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCasesPodsVerified
	namespace := "test-1"
	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Error(err.Error())
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
