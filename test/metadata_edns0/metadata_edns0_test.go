package metadataEdns0

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/ci/test/kubernetes"
)

func TestMetadata(t *testing.T) {

	corefileMeta := `.:53 {
       health
	   metadata
       metadata_edns0 {
          test 0xffee hex
       }
       log . "Meta: {/metadata_edns0/test}"
       forward . 8.8.8.8:53
}
`
	err := kubernetes.LoadCorefile(corefileMeta)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	err = kubernetes.StartClientPod("default")
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}

	ipMeta, err := kubernetes.CoreDNSPodIPs()
	if err != nil {
		t.Errorf("could not get coredns test pod ip: %v", err)
	}

	digCmd := "dig @" + ipMeta[0] + " google.com +ednsopt=65518:ABCDEF0123"
	_, err = kubernetes.Kubectl("-n default exec coredns-test-client -- " + digCmd)
	if err != nil {
		t.Fatalf("failed to execute query, got error: %s", err)
	}

	// Ensure that the logs have been collected before checking.
	logged := kubernetes.CorednsLogs()
	tries := 10
	for {
		if logged != "" {
			break
		}

		tries -= 1
		if tries == 0 {
			t.Errorf("Failed to get logs")
		}
		time.Sleep(500 * time.Millisecond)
		logged = kubernetes.CorednsLogs()
	}

	if !strings.Contains(logged, "Meta: abcdef0123") {
		t.Errorf("Expected it to contain: Meta: abcdef0123, got %v", logged)
	}
}
