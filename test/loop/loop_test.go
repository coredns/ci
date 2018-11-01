package loop

import (
	"testing"

	"github.com/coredns/ci/test/kubernetes"
	"strings"
)

func TestLoopDetected(t *testing.T) {

	corefile := `    .:53 {
        errors
        log
        loop
        kubernetes cluster.local
        proxy . 127.0.0.1
    }
`

    err := kubernetes.LoadCorefile(corefile)
	if err != nil && !strings.Contains(err.Error(), "loop detected") {
		t.Fatalf("Expected to see a \"loop detected\" error, got %s", err)
	}
}

func TestLoopBlackHoleUpstream(t *testing.T) {

	// 240.0.0.0 is a "reserved for future use" range that routers apparently drop.
	corefile := `    .:53 {
        errors
        log
        loop
        kubernetes cluster.local
        proxy . 240.0.0.0
    }
`

	err := kubernetes.LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
}
