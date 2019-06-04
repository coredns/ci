package kubernetes

import (
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/test"
	intTest "github.com/coredns/coredns/test"

	"github.com/miekg/dns"
)

var tests = []test.Case{
	{
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      303    IN      A       10.96.0.100"),
		},
	},
}

func TestKubernetesSecureAPI(t *testing.T) {

	endpointIP, err := Kubectl("get nodes -o jsonpath='{.items[*].status.addresses[?(@.type==\"InternalIP\")].address}'")
	if err != nil {
		t.Fatalf("Could not get Kubernetes Cluster IP: %s", err)
	}
	corefile :=
		`.:0 {
    kubernetes cluster.local {
        endpoint https://` + endpointIP + `:8443
        tls /home/circleci/.minikube/client.crt /home/circleci/.minikube/client.key /home/circleci/.minikube/ca.crt 
    }`

	server, udp, _, err := intTest.CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer server.Stop()

	// Work-around for timing condition that results in no-data being returned in test environment.
	time.Sleep(3 * time.Second)

	for _, tc := range tests {

		c := new(dns.Client)
		m := tc.Msg()

		res, _, err := c.Exchange(m, udp)
		if err != nil {
			t.Fatalf("Could not send query: %s", err)
		}
		if err := test.SortAndCheck(res, tc); err != nil {
			t.Error(err)
		}
	}
}
