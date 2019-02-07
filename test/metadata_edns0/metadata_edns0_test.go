package metadataEdns0

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/ci/test/kubernetes"
)

func TestMetadata(t *testing.T) {

	corefileMeta := `.:53 {
	   metadata
       metadata_edns0 {
          test 0xffee bytes
       }
       kubernetes cluster.local {
          pods insecure
       }
       log . "Meta: {/metadata_edns0/test}"
       forward . 8.8.8.8:53
}
`
	err := kubernetes.LoadCorefile(corefileMeta)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	ips, err := kubernetes.CoreDNSPodIPs("kube-dns")
	if err != nil {
		t.Errorf("could not get coredns pod ips: %v", err)
	}

	corefile := `.:53 {
        errors
        rewrite edns0 local set 0xffee hello-world
		forward . ` + ips[0] + `
}
`
	err = kubernetes.LoadCoreDNSTestCorefile(corefile, "coredns-test")
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	err = kubernetes.StartClientPod("default")
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}

	ipMeta, err := kubernetes.CoreDNSPodIPs("coredns-test")
	if err != nil {
		t.Errorf("could not get coredns test pod ip: %v", err)
	}

	digCmd := "dig @" + ipMeta[0] + " google.com"
	_, err = kubernetes.Kubectl("-n default exec coredns-test-client -- " + digCmd)
	if err != nil {
		t.Fatalf("failed to execute query, got error: %s", err)
	}

	time.Sleep(1 * time.Second)
	logged := kubernetes.CorednsLogs()
	if !strings.Contains(logged, "Meta: hello-world") {
		t.Errorf("Expected it to contain: Meta: hello-world.")
	}
}
