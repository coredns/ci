package kubernetes

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

var dnsTestCasesFallthrough = []test.Case{
	{
		Qname: "ext-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("example.net.		303	IN	A	13.14.15.16"),
			test.CNAME("ext-svc.test-1.svc.cluster.local. 303 IN	CNAME	example.net."),
		},
	},
	{
		Qname: "f.b.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("f.b.svc.cluster.local.      303    IN      A       10.10.10.11"),
		},
		Ns: []dns.RR{
			test.NS("cluster.local.	303	IN	NS	a.iana-servers.net."),
			test.NS("cluster.local.	303	IN	NS	b.iana-servers.net."),
		},
	},
	{
		Qname: "foo.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("foo.cluster.local.      303    IN      A       10.10.10.10"),
		},
		Ns: []dns.RR{
			test.NS("cluster.local.	303	IN	NS	a.iana-servers.net."),
			test.NS("cluster.local.	303	IN	NS	b.iana-servers.net."),
		},
	},
}

func TestKubernetesFallthrough(t *testing.T) {

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	time.Sleep(1 * time.Second)
	corefile := `    .:53 {
      health
      ready
	  errors
	  log
      file /etc/coredns/Zonefile cluster.local
      kubernetes cluster.local {
          namespaces test-1
          fallthrough
      }
      forward . ` + udp + `
    }
`
	err := LoadCorefileAndZonefile(corefile, clusterLocal, true)
	if err != nil {
		t.Fatalf("Could not load corefile/zonefile: %s", err)
	}
	testCases := dnsTestCasesFallthrough
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

var dnsTestCasesFallthroughFiltered = []test.Case{
	{
		Qname: "f.b.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("f.b.svc.cluster.local.      303    IN      A       10.10.10.11"),
		},
		Ns: []dns.RR{
			test.NS("cluster.local.	303	IN	NS	a.iana-servers.net."),
			test.NS("cluster.local.	303	IN	NS	b.iana-servers.net."),
		},
	},
	{
		Qname: "foo.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
}

func TestKubernetesFallthroughFiltered(t *testing.T) {
	corefile := `    .:53 {
      health
      ready
	  errors
	  log
      file /etc/coredns/Zonefile cluster.local
      kubernetes cluster.local {
          namespaces test-1
          fallthrough svc.cluster.local
      }
    }
`
	err := LoadCorefileAndZonefile(corefile, clusterLocal, true)
	if err != nil {
		t.Fatalf("Could not load corefile/zonefile: %s", err)
	}
	testCases := dnsTestCasesFallthroughFiltered
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

const clusterLocal = `    ; cluster.local test file for fallthrough
    cluster.local.		IN	SOA	sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600
    cluster.local.		IN	NS	b.iana-servers.net.
    cluster.local.		IN	NS	a.iana-servers.net.
    cluster.local.		IN	A	127.0.0.1
    cluster.local.		IN	A	127.0.0.2
    foo.cluster.local.      IN      A	10.10.10.10
    f.b.svc.cluster.local.  IN      A	10.10.10.11
    *.w.cluster.local.      IN      TXT     "Wildcard"
    a.b.svc.cluster.local.  IN      TXT     "Not a wildcard"
    cname.cluster.local.    IN      CNAME   www.example.net.
    service.namespace.svc.cluster.local.    IN      SRV     8080 10 10 cluster.local.
`
