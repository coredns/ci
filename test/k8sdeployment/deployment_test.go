package k8sdeployment

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/coredns/ci/test/kubernetes"
	metrics "github.com/coredns/coredns/plugin/metrics/test"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

var deploymentDNSCases = []test.Case{
	{ // A query for an existing service should return a record
		Qname: "svc-1-a.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("svc-1-a.test-1.svc.cluster.local.      5    IN      A       10.0.0.100"),
		},
	},
	{ // A query for an ip-style pod dns name should return a record
		Qname: "10-20-0-101.test-1.pod.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("10-20-0-101.test-1.pod.cluster.local. 303 IN A    10.20.0.101"),
		},
	},
	{ // A PTR record query for an existing service should return a record
		Qname: "100.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("100.0.0.10.in-addr.arpa. 303	IN	PTR	svc-1-a.test-1.svc.cluster.local."),
		},
	},
	{ // A PTR record query for an existing endpoint should return a record
		Qname: "253.0.17.172.in-addr.arpa.", Qtype: dns.TypePTR,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.PTR("253.0.17.172.in-addr.arpa. 303	IN	PTR	172-17-0-253.svc-1-a.test-1.svc.cluster.local."),
		},
	},
}

// Fuzzy cases compared for cardinality only
var deploymentDNSCasesFuzzy = []test.Case{
	{ // A query for an externalname service should return a CNAME and upstream A record
		Qname: "ext-svc.test-1.svc.cluster.local.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("example.net.      5    IN      A       1.2.3.4"),
			test.CNAME("ext-svc.test-1.svc.cluster.local.      5    IN      CNAME       example.net."),
		},
	},
	{ // A query for a name outside of k8s zone should get an answer via proxy
		Qname: "coredns.io.", Qtype: dns.TypeA,
		Rcode: dns.RcodeSuccess,
		Answer: []dns.RR{
			test.A("coredns.io.      5    IN      A       5.6.7.8"),
		},
	},
}

func TestKubernetesDeployment(t *testing.T) {

	t.Run("Deploy_with_deploy.sh", func(t *testing.T) {
		// Apply manifests via coredns/deployment deployment script ...
		path := os.Getenv("DEPLOYMENTPATH")
		cmd := exec.Command("sh", "-c", "./deploy.sh -i 10.0.0.10 -r 10.0.0.0/8 -r 172.17.0.0/16 | kubectl apply -f -")
		cmd.Dir = path + "/kubernetes"
		cmdout, err := cmd.CombinedOutput()
		print(string(cmdout))
		if err != nil {
			t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
		}
	})

	if false {
		t.Run("Verify_coredns_starts", func(t *testing.T) {
			maxWait := 120
			if kubernetes.WaitNReady(maxWait, 2) != nil {
				t.Fatalf("coredns failed to start in %v seconds,\nlog: %v", maxWait, kubernetes.CorednsLogs())
			}
		})

		t.Run("Verify_coredns_healthy", func(t *testing.T) {
			timeout := time.Second * time.Duration(90)

			ips, err := kubernetes.CoreDNSPodIPs()
			if err != nil {
				t.Errorf("could not get coredns pod ips: %v", err)
			}
			if len(ips) != 2 {
				t.Errorf("Expected 2 pods, found: %v", len(ips))
			}
			for _, ip := range ips {
				start := time.Now()
				for {
					resp, err := http.Get("http://" + ip + ":8080/health")
					if err != nil {
						t.Logf("pod (%v) healthy check error %v", ip, err)
						time.Sleep(time.Second)
						continue
					}

					// Any code greater than or equal to 200 and less than 400 indicates success.
					// Any other code indicates failure.
					if resp.StatusCode >= 200 && resp.StatusCode < 400 {
						break
					}

					if time.Since(start) >= timeout {
						t.Errorf("pod (%v) was not healthy in %v", ip, timeout)
						break
					}
					time.Sleep(time.Second)
				}
			}
		})

		t.Run("Verify_metrics_available", func(t *testing.T) {
			ips, err := kubernetes.CoreDNSPodIPs()
			if err != nil {
				t.Errorf("could not get coredns pod ips: %v", err)
			}
			if len(ips) != 2 {
				t.Errorf("Expected 2 pods, found: %v", len(ips))
			}
			for _, ip := range ips {
				mf := metrics.Scrape(t, "http://"+ip+":9153/metrics")
				if len(mf) == 0 {
					t.Errorf("unable to scrape metrics from %v", ip)
				}
			}
		})
	}
	// Verify dns query test strict cases
	testCases := deploymentDNSCases
	namespace := "test-1"
	err := kubernetes.StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := kubernetes.DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			test.CNAMEOrder(t, res)
			test.SortAndCheck(t, res, tc)
			if t.Failed() {
				t.Errorf("coredns log: %s", kubernetes.CorednsLogs())
			}
		})
	}
	// Verify dns query test fuzzy cases
	testCases = deploymentDNSCasesFuzzy
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.Qname, dns.TypeToString[tc.Qtype]), func(t *testing.T) {
			res, err := kubernetes.DoIntegrationTest(tc, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}
			test.CNAMEOrder(t, res)
			// Just compare the cardinality of the response to expected
			if len(tc.Answer) != len(res.Answer) {
				t.Errorf("Expected %v answers, got %v. coredns log: %s", len(tc.Answer), len(res.Answer), kubernetes.CorednsLogs())
			}
		})
	}
}
