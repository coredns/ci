package loop

import (
	"strings"
	"testing"
	"time"

	"github.com/coredns/ci/test/kubernetes"
)

// TestLoopDetected verifies that forwarding to self will be detected as a loop
func TestLoopDetected(t *testing.T) {

	corefile := `    .:53 {
        errors
        log
        loop
        proxy . 127.0.0.1
    }
`

	err := kubernetes.LoadCorefile(corefile)
	if err != nil && !strings.Contains(err.Error(), "loop detected") {
		t.Fatalf("Expected to see a \"loop detected\" error, got %s", err)
	}
}

// TestLoopBlackHoleUpstream verifies that a dead upstream will not cause loop to falsely detect a loop
func TestLoopBlackHoleUpstream(t *testing.T) {
	// 240.0.0.0 is a "reserved for future use" range that routers apparently drop.
	corefile := `    .:53 {
        errors
        log
        loop
        proxy . 240.0.0.0
    }
`

	err := kubernetes.LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
	time.Sleep(time.Second * 10)
	logs := kubernetes.CorednsLogs()
	if strings.Contains(logs, "loop detected") {
		t.Fatalf("Did not expect to see \"loop detected\" in logs, got %s", logs)
	}
}
