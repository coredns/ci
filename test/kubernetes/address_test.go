package kubernetes

import (
	"fmt"
	"os"
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
	{ // By default, pod queries are disabled, so a pod query should return NXDOMAIN
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.        303     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 30"),
		},
	},
	{ // A TXT request for dns-version should return the version of the kubernetes service discovery spec implemented
		Qname: "dns-version.cluster.local.", Qtype: dns.TypeTXT,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.TXT(`dns-version.cluster.local. 303 IN TXT "1.1.0"`),
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
	{ // A query to a headless service with unready endpoints should return NXDOMAIN
		Qname: "svc-unready.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeNameError,
		Ns: []dns.RR{
			test.SOA("cluster.local.        303     IN      SOA     ns.dns.cluster.local. hostmaster.cluster.local. 1499347823 7200 1800 86400 30"),
		},
	},
	{ // An NS type query
		Qname: "cluster.local.", Qtype: dns.TypeNS,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.NS("cluster.local.	5	IN	NS	kube-dns.kube-system.svc.cluster.local."),
		},
		Extra: []dns.RR{
			test.A("kube-dns.kube-system.svc.cluster.local. 5 IN A 10.96.0.10"),
		},
	},
}

var newObjectTests = []test.Case{
	{
		Qname: "new-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("new-svc.test-1.svc.cluster.local.      5    IN      A       10.96.0.222"),
		},
	},
	{
		Qname: "172-17-0-222.new-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("172-17-0-222.new-svc.test-1.svc.cluster.local.      5    IN      A       172.17.0.222"),
		},
	},
}

var newObjects = `apiVersion: v1
kind: Service
metadata:
  name: new-svc
  namespace: test-1
spec:
  clusterIP: 10.96.0.222
  ports:
  - name: http
    port: 80
    protocol: TCP
---
kind: Endpoints
apiVersion: v1
metadata:
  name: new-svc
  namespace: test-1
subsets:
  - addresses:
      - ip: 172.17.0.222
    ports:
      - port: 80
        name: http
        protocol: TCP
`

func TestKubernetesA(t *testing.T) {

	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `    .:53 {
        health
        ready
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
			namespaces test-1
		}
		forward . ` + udp + `
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
			test.CNAMEOrder(res)
			if err := test.SortAndCheck(res, tc); err != nil {
				t.Error(err)
			}
			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}

	newObjectsFile, rmFunc, err := test.TempFile(os.TempDir(), newObjects)
	defer rmFunc()
	if err != nil {
		t.Fatalf("could not create file to add service/endpoint: %s", err)
	}

	_, err = Kubectl("apply -f " + newObjectsFile)
	if err != nil {
		t.Fatalf("could not add service/endpoint via kubectl: %s", err)
	}

	for _, tc := range newObjectTests {
		t.Run("New Object "+tc.Qname, func(t *testing.T) {
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

	_, err = Kubectl("-n test-1 delete service new-svc")
	if err != nil {
		t.Fatalf("could not add service/endpoint via kubectl: %s", err)
	}
}

var dnsTestLongHeadlessSvc = `apiVersion: v1
apiVersion: v1
kind: Service
metadata:
  name: headless-long-svc
  namespace: test-1
spec:
  clusterIP: None
  ports:
  - name: c-port
    port: 1235
    protocol: UDP
---
kind: Endpoints
apiVersion: v1
metadata:
  name: headless-long-svc
  namespace: test-1
subsets:
  - addresses:
      - ip: 172.17.0.100
      - ip: 172.17.0.101
      - ip: 172.17.0.102
      - ip: 172.17.0.103
      - ip: 172.17.0.104
      - ip: 172.17.0.105
      - ip: 172.17.0.106
      - ip: 172.17.0.107
      - ip: 172.17.0.108
      - ip: 172.17.0.109
      - ip: 172.17.0.110
      - ip: 172.17.0.111
      - ip: 172.17.0.112
      - ip: 172.17.0.113
      - ip: 172.17.0.114
      - ip: 172.17.0.115
      - ip: 172.17.0.116
      - ip: 172.17.0.117
      - ip: 172.17.0.118
      - ip: 172.17.0.119
      - ip: 172.17.0.120
      - ip: 172.17.0.121
      - ip: 172.17.0.122
      - ip: 172.17.0.123
      - ip: 172.17.0.124
      - ip: 172.17.0.125
      - ip: 172.17.0.126
      - ip: 172.17.0.127
      - ip: 172.17.0.128
      - ip: 172.17.0.129
      - ip: 172.17.0.130
      - ip: 172.17.0.131
      - ip: 172.17.0.132
      - ip: 172.17.0.133
      - ip: 172.17.0.134
      - ip: 172.17.0.135
    ports:
      - port: 1235
        name: c-port
        protocol: UDP
`

func TestTCFlagForHeadlessService(t *testing.T) {
	rmFunc, upstream, udp := UpstreamServer(t, "example.net", ExampleNet)
	defer upstream.Stop()
	defer rmFunc()

	corefile := `
        .:53 {
            health
            ready
            errors
            log
            kubernetes cluster.local 10.in-addr.arpa {
                namespaces test-1
            }
            forward . ` + udp + `
        }
    `
	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	namespace := "test-1"
	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("Failed to start client pod: %s", err)
	}

	newLongHeadlessSvcObjectsFile, rmFunc, err := test.TempFile(os.TempDir(), dnsTestLongHeadlessSvc)
	defer rmFunc()
	if err != nil {
		t.Fatalf("Could not create file to add headless service/endpoint: %s", err)
	}
	_, err = Kubectl("apply -f " + newLongHeadlessSvcObjectsFile)
	if err != nil {
		t.Fatalf("Could not add headless service/endpoint via kubectl: %s", err)
	}

	t.Run("Test long headless service: with buffer size 512", func(t *testing.T) {
		testHeadlessService(t, "headless-long-svc.test-1.svc.cluster.local.", dns.TypeA, dns.RcodeSuccess, "512", true, namespace)
	})
	t.Run("Test long headless service: with buffer size 659", func(t *testing.T) {
		testHeadlessService(t, "headless-long-svc.test-1.svc.cluster.local.", dns.TypeA, dns.RcodeSuccess, "659", false, namespace)
	})
	t.Run("Test long headless service: with buffer size 1024", func(t *testing.T) {
		testHeadlessService(t, "headless-long-svc.test-1.svc.cluster.local.", dns.TypeA, dns.RcodeSuccess, "1024", false, namespace)
	})

	_, err = Kubectl("-n test-1 delete service headless-long-svc")
	if err != nil {
		t.Fatalf("Could not delete service/endpoint via kubectl: %s", err)
	}
}

func testHeadlessService(t *testing.T, qname string, qtype uint16, expectedRcode int, buffSize string, expectTruncated bool, namespace string) {
	t.Helper()
	res, err := DoIntegrationTestWithUDPBufSize(test.Case{
		Qname: qname,
		Qtype: qtype,
		Rcode: expectedRcode,
	}, namespace, buffSize)

	if err != nil {
		t.Errorf(err.Error())
	}
	if res.Rcode != expectedRcode {
		t.Errorf("Rcode is %q, expected %q", dns.RcodeToString[res.Rcode], dns.RcodeToString[expectedRcode])
	}
	if res.Truncated != expectTruncated {
		t.Errorf("TC flag is %v, expected %v", res.Truncated, expectTruncated)
	}
	if t.Failed() {
		t.Errorf("CoreDNS log: %s", CorednsLogs())
	}
}
