package k8sdeployment

import (
	"os/exec"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
)

// This test restarts the kube-APIserver and checks if CoreDNS connection to the API
// is valid and there are no failures.
// This test is to catch bugs/errors such as the one reported in https://github.com/coredns/coredns/issues/2464
func TestConnectionAfterAPIRestart(t *testing.T) {

	t.Skip("Test needs to be refactored for kind environment")
	return

	// Apply manifests via coredns/deployment deployment script ...
	cmd := exec.Command("sh", "-c", " ~/go/src/${CIRCLE_PROJECT_USERNAME}/deployment/kubernetes/deploy.sh -s -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl apply -f -")
	cmdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
	}

	// Verify that the CoreDNS pods are up and ready.
	maxWait := 120
	if kubernetes.WaitNReady(maxWait, 2) != nil {
		t.Fatalf("coredns failed to start in %v seconds,\nlog: %v", maxWait, kubernetes.CorednsLogs())
	}

	// Check if CoreDNS has restarted before the APIServer restart
	restartCount, err := kubernetes.HasCoreDNSRestarted()
	if err != nil {
		t.Fatalf("error fetching CoreDNS pod restart count: %s", err)
	}
	if restartCount {
		t.Fatalf("error as CoreDNS has crashed: %s.", kubernetes.CorednsLogs())
	}

	// Restart the Kubernetes APIServer.
	dockerCmd, err := exec.Command("sh", "-c", "docker restart $(docker ps --no-trunc | grep 'kube-apiserver' | awk '{ print $1; }') > /dev/null").CombinedOutput()
	if err != nil {
		t.Fatalf("docker container restart failed: %s\nerr: %s", string(dockerCmd), err)
	}
	// Verify that the CoreDNS pods are up and ready after the restart.
	maxWait = 120
	if kubernetes.WaitNReady(maxWait, 2) != nil {
		t.Fatalf("coredns failed to start in %v seconds,\nlog: %v", maxWait, kubernetes.CorednsLogs())
	}

	// Check if CoreDNS has crashed after APIServer restart
	restartCount, err = kubernetes.HasCoreDNSRestarted()
	if err != nil {
		t.Fatalf("error fetching CoreDNS pod restart count: %s", err)
	}

	// If CoreDNS crashes due to KubeAPIServer restart, Kubernetes will restart the CoreDNS containers.
	if restartCount {
		t.Fatalf("failed as CoreDNS crashed due to KubeAPIServer restart.")
	}
}
