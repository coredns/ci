package kubernetes

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

func TestKubernetesEndpointPodNames(t *testing.T) {
	var tests = []struct {
		test.Case
		TargetRegEx             string
		AnswerCount, ExtraCount int
	}{
		{
			Case:        test.Case{Qname: "headless-1.test-3.svc.cluster.local.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess},
			AnswerCount: 1,
			ExtraCount:  1,
			TargetRegEx: "^test-name-.+\\.headless-1\\.test-3\\.svc\\.cluster\\.local\\.$",
		},
		{
			Case:        test.Case{Qname: "headless-2.test-3.svc.cluster.local.", Qtype: dns.TypeSRV, Rcode: dns.RcodeSuccess},
			AnswerCount: 1,
			ExtraCount:  1,
			// The pod name selected by headless-2 exceeds the valid dns label length, so it should fallback to the dashed-ip
			TargetRegEx: "^[0-9]+-[0-9]+-[0-9]+-[0-9]+\\.headless-1\\.test-3\\.svc\\.cluster\\.local\\.$",
		},
	}

	// namespace test-3 contains headless services/deployments for this test.
	// enable endpoint_pod_names in Corefile
	corefile := `    .:53 {
        health
        ready
        errors
        log
        kubernetes cluster.local 10.in-addr.arpa {
			namespaces test-3
			endpoint_pod_names
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
	for _, expected := range tests {
		t.Run(fmt.Sprintf("%s %s", expected.Qname, dns.TypeToString[expected.Qtype]), func(t *testing.T) {
			result, err := DoIntegrationTest(expected.Case, namespace)
			if err != nil {
				t.Errorf(err.Error())
			}

			if len(result.Answer) != expected.AnswerCount {
				t.Errorf("Expected %v answers, got %v.", expected.AnswerCount, len(result.Answer))
			}
			if len(result.Extra) != expected.ExtraCount {
				t.Errorf("Expected %v additionals, got %v.", expected.ExtraCount, len(result.Extra))
			}
			if len(result.Answer) > 0 {
				match, err := regexp.Match(expected.TargetRegEx, []byte(result.Answer[0].(*dns.SRV).Target))
				if err != nil {
					t.Error(err)
				}
				if !match {
					t.Errorf("Answer target %q did not match regex %q", result.Answer[0].(*dns.SRV).Target, expected.TargetRegEx)
				}
			}
			if len(result.Extra) > 0 {
				match, err := regexp.Match(expected.TargetRegEx, []byte(result.Extra[0].Header().Name))
				if err != nil {
					t.Error(err)
				}
				if !match {
					t.Errorf("Extra name %q did not match regex %q", result.Extra[0].Header().Name, expected.TargetRegEx)
				}
			}

			if t.Failed() {
				t.Errorf("coredns log: %s", CorednsLogs())
			}
		})
	}
}
