package kubernetes

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var autopathTests = []test.Case{
	{ // Valid service name -> success on 1st search in path -> A record
		Qname: "svc-1-a", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
		},
	},
	{ // Valid service name + namespace -> success on 2nd search in path -> CNAME glue + A record
		Qname: "svc-1-a.test-1", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
			test.CNAME("svc-1-a.test-1.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // Valid service name + namespace + svc -> success on 3nd search in path -> CNAME glue + A record
		Qname: "svc-1-a.test-1.svc", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
			test.CNAME("svc-1-a.test-1.svc.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // Valid fqdn for internal service -> success on empty search -> CNAME glue + A record
		Qname: "svc-1-a.test-1.svc.cluster.local", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
			test.CNAME("svc-1-a.test-1.svc.cluster.local.test-1.svc.cluster.local.  303    IN	     CNAME	  svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // Valid external fqdn -> success on empty search -> CNAME glue + A record
		Qname: "foo.example.net", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("foo.example.net.      303    IN      A       10.10.10.11"),
			test.CNAME("foo.example.net.test-1.svc.cluster.local.  303    IN	     CNAME	  foo.example.net."),
		},
		Ns: []dns.RR{
			test.NS("example.net.	303	IN	NS	ns.example.net."),
		},
	},
	/*
		{ // prevent client search on query fail - this optimization is not implemented
			Qname: "bar.example.net", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
		},
	*/
}

func TestKubernetesAutopath(t *testing.T) {

	// set up server to handle internal zone, to trap *.internal search path in travis environment.
	internal := `; internal zone info for autopath tests
internal.		IN	SOA	sns.internal. noc.internal. 2015082541 7200 3600 1209600 3600
`
	rmFunc, upstream, udp := UpstreamServer(t, "internal", internal)
	defer upstream.Stop()
	defer rmFunc()

	corefile :=
		`    .:53 {
      health
      ready
      errors
      log
      autopath @kubernetes
      kubernetes cluster.local {
        pods verified
      }
      file /etc/coredns/Zonefile example.net
      forward internal ` + udp + `
    }
`
	exampleZonefile := `    ; example.net zone info for autopath tests
    example.net.		IN	SOA	sns.example.net. noc.example.net. 2015082541 7200 3600 1209600 3600
    example.net.		IN	NS	ns.example.net.
    example.net.      IN      A	10.10.10.10
    foo.example.net.      IN      A	10.10.10.11

`
	err := LoadCorefileAndZonefile(corefile, exampleZonefile, true)
	if err != nil {
		t.Fatalf("Could not load corefile/zonefile: %s", err)
	}
	testCases := autopathTests
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
			if res != nil {
				test.CNAMEOrder(res)
				if err := test.SortAndCheck(res, tc); err != nil {
					t.Error(err)
				}
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
