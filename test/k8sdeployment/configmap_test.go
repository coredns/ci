package k8sdeployment

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
)

func TestConfigMapTranslation(t *testing.T) {
	stubdata := `{"abc.com" : ["1.2.3.4:5300","4.4.4.4"], "my.cluster.local" : ["2.3.4.5:5300"]}`
	upstreamdata := `["8.8.8.8", "8.8.4.4"]`

	corefileExpected := `.:53 {
    errors
    health {
      lameduck 5s
    }
    ready
    kubernetes cluster.local  10.96.0.0/8 172.17.0.0/16 {
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . 8.8.8.8 8.8.4.4 {
      max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}
abc.com:53 {
  errors
  cache 30
  loop
  forward . 1.2.3.4:5300 {
    max_concurrent 1000
  }
}

my.cluster.local:53 {
  errors
  cache 30
  loop
  forward . 2.3.4.5:5300 {
    max_concurrent 1000
  }
}
`

	err := kubernetes.LoadKubednsConfigmap(stubdata, upstreamdata)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	// Apply Corefile translation via coredns/deployment deployment script
	cmd := exec.Command("sh", "-c", " ~/go/src/${CIRCLE_PROJECT_USERNAME}/deployment/kubernetes/deploy.sh -s -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl delete --ignore-not-found=true -f -")
	cmdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to delete deployment objects: %s\nerr: %s", string(cmdout), err)
	}

	cmd = exec.Command("sh", "-c", " ~/go/src/${CIRCLE_PROJECT_USERNAME}/deployment/kubernetes/deploy.sh -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl apply --overwrite=true -f -")
	cmdout, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
	}

	corefileTranslated, err := kubernetes.Kubectl("-n kube-system get configmap coredns -ojsonpath={.data.Corefile}")
	if err != nil {
		t.Fatalf("error fetching translated corefile: %s", err)
	}

	if strings.Compare(corefileTranslated, corefileExpected) != 0 {
		t.Fatalf("failed test: Translation does not match.\nGOT:\n" + corefileTranslated + "\n\nEXPECTED:\n" + corefileExpected)
	}

	// Clean-up by removing kube-dns ConfigMap
	_, err = kubernetes.Kubectl("-n kube-system delete cm kube-dns")
	if err != nil {
		t.Fatalf("error deleting kube-dns ConfigMap: %s", err)
	}
}
