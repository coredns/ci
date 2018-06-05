package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var dnsTestCasesA = []test.Case{
	{ // An A record query for an existing service should return a record
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      5    IN      A       10.96.0.100"),
		},
	},
	{ // An A record query for an existing headless service should return a record for each of its ipv4 endpoints
		Qname: "headless-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("headless-svc.test-1.svc.cluster.local.      5    IN      A       172.17.0.254"),
			test.A("headless-svc.test-1.svc.cluster.local.      5    IN      A       172.17.0.255"),
		},
	},
	{ // An A record query for a non-existing service should return NXDOMAIN
		Qname: "bogusservice.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // An A record query for a non-existing endpoint should return NXDOMAIN
		Qname: "bogusendpoint.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // A service record search with a wild card namespace should return all (1) services in exposed namespaces
		Qname: "svc-1-a.*.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.*.svc.cluster.local.      303    IN      A       10.96.0.100"),
		},
	},
	{ // A wild card service name in an exposed namespace should result in all records
		Qname: "*.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.96.0.110"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       10.96.0.115"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       172.17.0.254"),
			test.A("*.test-1.svc.cluster.local.      303    IN      A       172.17.0.255"),
			test.CNAME("*.test-1.svc.cluster.local.  303    IN      CNAME   example.net."),
			test.A("example.net.                     303    IN      A       13.14.15.16"),
		},
	},
	{ // A wild card service name in an un-exposed namespace result in nxdomain
		Qname: "*.test-2.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{ // By default, pod queries are disabled, so a pod query should return NXDMOAIN
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.        300     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 60"),
		},
	},
	{ // A TXT request for dns-version should return the version of the kubernetes service discovery spec implemented
		Qname: "dns-version.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.TXT(`dns-version.cluster.local. 303 IN TXT "1.0.1"`),
		},
	},
	{ // An AAAA record query for an existing headless service should return a record for each of its ipv6 endpoints
		Qname: "headless-svc.test-1.svc.cluster.local.", Qtype: dns.TypeAAAA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.AAAA("headless-svc.test-1.svc.cluster.local.      5    IN      AAAA      1234:abcd::1"),
			test.AAAA("headless-svc.test-1.svc.cluster.local.      5    IN      AAAA      1234:abcd::2"),
		},
	},
}

func TestKubernetesA(t *testing.T) {

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
            namespaces test-1
            upstream ` + udp + `
        }
    }
`

	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	testCases := dnsTestCasesA
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
