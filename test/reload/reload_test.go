package reload

import (
	"github.com/coredns/ci/test/kubernetes"
	"strings"
	"testing"
)

func TestReload(t *testing.T) {

	corefileReload := `.:53 {
       health
       reload 2s
       hosts {
         10.0.1.2 mytest.com
       }
}
`
	err := kubernetes.LoadCorefile(corefileReload)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	err = kubernetes.StartClientPod("default")
	if err != nil {
		t.Fatalf("failed to start client pod: %s", err)
	}

	digCmd := "dig mytest.com +short"
	cmdOut, err := kubernetes.Kubectl("-n default exec coredns-test-client -- " + digCmd)
	if err != nil {
		t.Fatalf("failed to execute query, got error: %s", err)
	}

	if !strings.Contains(cmdOut, "10.0.1.2") {
		t.Fatalf("coredns failed to load the Configmap")
	}

	// Patch the CoreDNS ConfigMap with a new Corefile.
	corefilePatchData := `patch configmap/coredns -n kube-system -p '{"data":{"Corefile":".:53 {\n       health\n       reload 2s\n       hosts {\n           10.0.1.20 mytest.com\n        }\n    }"}}'`
	_, err = kubernetes.Kubectl(corefilePatchData)
	if err != nil {
		t.Fatalf("could not patch the coredns ConfigMap via kubectl: %s", err)
	}

	logged := kubernetes.CorednsLogs()
	// check if CoreDNS was able to reload and hasn't crashed.
	maxWait := 120
	if kubernetes.WaitReady(maxWait) != nil {
		t.Fatalf("coredns crashed due to failed reload \nlog: %v", logged)
	}

	// check if CoreDNS is reloading only once.
	if strings.Count(logged, "[INFO] Reloading\n") > 1 {
		t.Fatalf("coredns tried to reload more than once")
	}

	cmdOut, err = kubernetes.Kubectl("-n default exec coredns-test-client -- " + digCmd)
	if err != nil {
		t.Fatalf("failed to execute query, got error: %s", err)
	}

	// check if the reload was successful and isn't using the previous ConfigMap.
	if !strings.Contains(cmdOut, "10.0.1.20") {
		t.Fatalf("coredns failed to reload and is using the previous Configmap")
	}

}
