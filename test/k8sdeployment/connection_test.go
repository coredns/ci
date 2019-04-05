package k8sdeployment

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coredns/ci/test/kubernetes"
)

// This test restarts the kube-APIserver and checks if CoreDNS connection to the API
// is valid and there are no failures.
// This test is to catch bugs/errors such as the one reported in https://github.com/coredns/coredns/issues/2464
func TestConnectionAfterAPIRestart(t *testing.T) {
	// Apply manifests via coredns/deployment deployment script ...
	path := os.Getenv("DEPLOYMENTPATH")
	cmd := exec.Command("sh", "-c", "./deploy.sh -s -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl apply -f -")
	cmd.Dir = path + "/kubernetes"
	cmdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
	}

	// Restart the Kubernetes APIserver and wait for it to come back up.
	dockerCmd, err := exec.Command("sh", "-c", "docker restart $(docker ps --no-trunc | grep 'kube-apiserver' | awk '{ print $1; }' > /dev/null").CombinedOutput()
	if err != nil {
		t.Fatalf("docker container restart failed: %s\nerr: %s", string(dockerCmd), err)
	}
	time.Sleep(10 * time.Second)

	// Get the restart count of the CoreDNS pods.
	restartCount, err := kubernetes.Kubectl("-n kube-system get pods -l k8s-app=kube-dns -ojsonpath='{.items[*].status.containerStatuses[0].restartCount}'")
	if err != nil {
		t.Fatalf("error fetching CoreDNS pod restart count: %s", err)
	}

	// If CoreDNS crashes due to KubeAPIServer restart, Kubernetes will restart the CoreDNS containers.
	if !strings.Contains(restartCount, "0") {
		t.Fatalf("failed as CoreDNS crashed due to KubeAPIServer restart.")
	}
}
