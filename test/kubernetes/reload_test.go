package kubernetes

import (
	"strings"
	"testing"
	"time"
)

func TestReload(t *testing.T) {

	corefile := `    .:53 {
        health
        ready
        errors
        log
        reload
        kubernetes cluster.local
    }
`
	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	err = StartClientPod(namespace)
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}

	// change the configmap
	corefile = `    .:53 {
        health
        ready
        errors
        log
        reload 2s
        kubernetes cluster.local test
    }
`
	err = LoadCorefileAndZonefile(corefile, "", false)
	if err != nil {
		t.Fatalf("failed to update Corefile: %s", err)
	}

	// wait for relead to happen (can take up to 2 minutes for Configmap change to be propagated to coredns pod)
	for i := 0; i < 120; i++ {
		logs := CorednsLogs()

		if strings.Contains(logs, "[PANIC]") {
			t.Fatalf("detected panic: %s", logs)
		}
		if strings.Contains(logs, "[INFO] Reloading complete") {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("timed out waiting for reload")
}
