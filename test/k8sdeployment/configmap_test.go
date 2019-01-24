package k8sdeployment

import (
	"os"
	"os/exec"
	"testing"

	"github.com/coredns/ci/test/kubernetes"
	"strings"
)

func TestConfigMapTranslation(t *testing.T) {
	feddata := `{"foo" : "foo.fed.com", "bar.com" : "bar.fed.com"}`
	stubdata := `{"abc.com" : ["1.2.3.4:5300","4.4.4.4"], "my.cluster.local" : ["2.3.4.5:5300"]}`
	upstreamdata := `["8.8.8.8", "8.8.4.4"]`

	corefileExpected := `.:53 {
    errors
    health
    kubernetes cluster.local  10.96.0.0/8 172.17.0.0/16 {
      pods insecure
      upstream
      fallthrough in-addr.arpa ip6.arpa
    }
    federation {
      foo foo.fed.com
      bar.com bar.fed.com
    }
    prometheus :9153
    proxy . 8.8.8.8 8.8.4.4
    cache 30
    loop
    reload
    loadbalance
}
abc.com:53 {
  errors
  cache 30
  loop
  proxy . 1.2.3.4:5300
}

my.cluster.local:53 {
  errors
  cache 30
  loop
  proxy . 2.3.4.5:5300
}
`

	err := kubernetes.LoadKubednsConfigmap(feddata, stubdata, upstreamdata)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	// Apply Corefile translation via coredns/deployment deployment script
	path := os.Getenv("DEPLOYMENTPATH")
	cmd := exec.Command("sh", "-c", "./deploy.sh -i 10.96.0.10 -r 10.96.0.0/8 -r 172.17.0.0/16 | kubectl apply -f -")
	cmd.Dir = path + "/kubernetes"
	cmdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deployment script failed: %s\nerr: %s", string(cmdout), err)
	}

	corefileTranslated, err := kubernetes.Kubectl("-n kube-system get configmap coredns -ojsonpath={.data.Corefile}")
	if err != nil {
		t.Fatalf("error fetching translated corefile: %s", err)
	}

	if strings.Compare(corefileTranslated, corefileExpected) != 0 {
		t.Fatalf("failed test: Translation does not match")
	}
}
