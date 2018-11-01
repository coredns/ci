package loop

import (
	"testing"

	"github.com/coredns/ci/test/internalplugins/kubernetes"
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

func TestLoopBadUpstream(t *testing.T) {

	// 10.0.0.1:9999 is an invalid upstream server
	corefile := `    .:53 {
        errors
        log
        loop
        kubernetes cluster.local
        proxy . 10.96.0.1:9999
    }
`

	err := kubernetes.LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}
}
