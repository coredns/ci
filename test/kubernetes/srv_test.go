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
		Qname: "*._TcP.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*._TcP.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	 0  50  443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._TcP.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	 0  50   80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303 	IN	A	10.96.0.100"),
		},
	},
	{
		Qname: "*.*.bogusservice.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.any.svc-1-a.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*.any.svc-1-a.*.svc.cluster.local.      303    IN    SRV 0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*.any.svc-1-a.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.96.0.100"),
		},
	},
	{
		Qname: "ANY.*.svc-1-a.any.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("ANY.*.svc-1-a.any.svc.cluster.local.      303    IN    SRV 0 50 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("ANY.*.svc-1-a.any.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303 	IN	A	10.96.0.100"),
		},
	},
	{
		Qname: "*.*.bogusservice.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.*.bogusservice.any.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "_c-port._UDP.*.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.        303       IN      SRV     0 20 1234 1234-abcd--1.headless-svc.test-1.svc.cluster.local."),
			test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.        303       IN      SRV     0 20 1234 1234-abcd--2.headless-svc.test-1.svc.cluster.local."),
			test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.      303    IN    SRV 0 20 1234 172-17-0-254.headless-svc.test-1.svc.cluster.local."),
			test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.      303    IN    SRV 0 20 1234 172-17-0-255.headless-svc.test-1.svc.cluster.local."),
			test.SRV("_c-port._UDP.*.test-1.svc.cluster.local.      303    IN    SRV 0 20 1234 svc-c.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.AAAA("1234-abcd--1.headless-svc.test-1.svc.cluster.local.     303       IN      AAAA    1234:abcd::1"),
			test.AAAA("1234-abcd--2.headless-svc.test-1.svc.cluster.local.     303       IN      AAAA    1234:abcd::2"),
			test.A("172-17-0-254.headless-svc.test-1.svc.cluster.local.	303	IN	A	172.17.0.254"),
			test.A("172-17-0-255.headless-svc.test-1.svc.cluster.local.	303	IN	A	172.17.0.255"),
			test.A("svc-c.test-1.svc.cluster.local.	303	IN	A	10.96.0.115"),
		},
	},
	{
		Qname: "*._tcp.any.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 443 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 80  svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*._tcp.any.test-1.svc.cluster.local.      303    IN    SRV 0 33 80  svc-1-b.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.96.0.100"),
			test.A("svc-1-b.test-1.svc.cluster.local.	303	IN	A	10.96.0.110"),
		},
	},
	{
		Qname: "*.*.any.test-2.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "*.*.*.test-2.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
	{
		Qname: "_http._tcp.*.*.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("_http._tcp.*.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-a.test-1.svc.cluster.local."),
			test.SRV("_http._tcp.*.*.svc.cluster.local.      303    IN    SRV 0 50 80 svc-1-b.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.	303	IN	A	10.96.0.100"),
			test.A("svc-1-b.test-1.svc.cluster.local.	303	IN	A	10.96.0.110"),
		},
	},
	{
		Qname: "*.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.SRV("*.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 443 172-17-0-253.svc-1-a.test-1.svc.cluster.local."),
			test.SRV("*.svc-1-a.test-1.svc.cluster.local.	303	IN	SRV	0 50 80 172-17-0-253.svc-1-a.test-1.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("172-17-0-253.svc-1-a.test-1.svc.cluster.local.	303	IN	A	172.17.0.253"),
		},
	},
	{
		Qname: "*._not-udp-or-tcp.svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeSRV,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.	303	IN	SOA	ns.dns.cluster.local. hostmaster.cluster.local. 1502313310 7200 1800 86400 60"),
		},
	},
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
	    errors
	    log
        kubernetes cluster.local {
            namespaces test-1
            upstream ` + udp + `
        }
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
			test.CNAMEOrder(t, res)
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
