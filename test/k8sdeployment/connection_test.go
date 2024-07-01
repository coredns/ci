package k8sdeployment

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
)

// This test restarts the kube-APIserver and checks if CoreDNS connection to the API
// is valid and there are no failures.
// This test is to catch bugs/errors such as the one reported in https://github.com/coredns/coredns/issues/2464
func TestConnectionAfterAPIRestart(t *testing.T) {
	// Apply manifests via coredns/deployment deployment script ...
	cmd := exec.Command("sh", "-c", " ~/go/src/${CIRCLE_PROJECT_USERNAME}/deployment/kubernetes/deploy.sh -s -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl delete --ignore-not-found=true -f -")
	cmdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to delete deployment objects: %s\nerr: %s", string(cmdout), err)
	}

	cmd = exec.Command("sh", "-c", " ~/go/src/${CIRCLE_PROJECT_USERNAME}/deployment/kubernetes/deploy.sh -s -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl apply -f -")
	cmdout, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
	}

	// Verify that the CoreDNS pods are up and ready.
	maxWait := 120
	if kubernetes.WaitNReady(maxWait, 1) != nil {
		t.Fatalf("coredns failed to start in %v seconds,\nlog: %v", maxWait, kubernetes.CorednsLogs())
	}

	// Check if CoreDNS has restarted before the APIServer restart
	hasCoreDNSRestarted, err := kubernetes.HasResourceRestarted(kubernetes.CoreDNSLabel)
	if err != nil {
		t.Fatalf("error fetching CoreDNS pod restart count: %s", err)
	}
	if hasCoreDNSRestarted {
		t.Fatalf("error as CoreDNS has crashed: %s.", kubernetes.CorednsLogs())
	}

	// Restart the Kubernetes APIServer.
	containerID, err := kubernetes.FetchDockerContainerID("kind-control-plane")
	if err != nil {
		t.Fatalf("docker container ID not found, err: %s", err)
	}

	dockerCmd := fmt.Sprintf("docker exec -i %s /bin/sh -c \"pkill kube-apiserver\"", containerID)
	killAPIServer, err := exec.Command("sh", "-c", dockerCmd).CombinedOutput()
	if err != nil {
		t.Fatalf("API Server restart failed: %s\nerr: %s", string(killAPIServer), err)
	}

	// Verify that the CoreDNS pods are up and ready after the restart.
	maxWait = 120
	if kubernetes.WaitNReady(maxWait, 1) != nil {
		t.Fatalf("coredns failed to start in %v seconds,\nlog: %v", maxWait, kubernetes.CorednsLogs())
	}

	// Check if the api-server was actually restarted
	hasAPIRestarted, err := kubernetes.HasResourceRestarted(kubernetes.APIServerLabel)
	if err != nil {
		t.Fatalf("error fetching kube-apiserver pod restart count: %s", err)
	}
	if !hasAPIRestarted {
		t.Fatalf("API Server restart failed")
	}

	// Check if CoreDNS has crashed after APIServer restart
	hasCoreDNSRestarted, err = kubernetes.HasResourceRestarted(kubernetes.CoreDNSLabel)
	if err != nil {
		t.Fatalf("error fetching CoreDNS pod restart count: %s", err)
	}

	// If CoreDNS crashes due to KubeAPIServer restart, Kubernetes will restart the CoreDNS containers.
	if hasCoreDNSRestarted {
		t.Fatalf("failed as CoreDNS crashed due to KubeAPIServer restart.")
	}
}
