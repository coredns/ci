package kubernetes

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"
	intTest "github.com/coredns/coredns/test"

	"github.com/miekg/dns"
	"time"
)

var multiK8sCases = []test.Case{
	{ // An A record query for an existing service should return a record
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      10    IN      A       10.0.0.100"),
		},
	},
	{ // An A record query for an existing service should return a record
		Qname: "kubernetes.default.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("kubernetes.default.svc.cluster.local.      20    IN      A       10.0.0.1"),
		},
	},
}

func TestMultiKubernetes(t *testing.T) {

	corefile :=
		`.:0 {
    kubernetes cluster.local {
        ttl 10
        namespaces test-1
        endpoint https://127.0.0.1:8443
        tls /root/.minikube/client.crt /root/.minikube/client.key /root/.minikube/ca.crt
        fallthrough
    }
    kubernetes cluster.local {
        ttl 20
        namespaces default
        endpoint https://127.0.0.1:8443
        tls /root/.minikube/client.crt /root/.minikube/client.key /root/.minikube/ca.crt 
    }
}`

	server, udp, _, err := intTest.CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer server.Stop()

	// Work-around for timing condition that results in no-data being returned in test environment.
	time.Sleep(3 * time.Second)

	for _, tc := range multiK8sCases {

		c := new(dns.Client)
		m := tc.Msg()

		res, _, err := c.Exchange(m, udp)
		if err != nil {
			t.Errorf("Could not send query: %s", err)
		}
		test.SortAndCheck(t, res, tc)
	}
}
