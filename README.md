# ci

Repository for continuous integration tests for the CoreDNS project. Currently tests relating to Kubernetes are contained here, but integration tests for other internal CoreDNS plugins may be moved here or added here in the future. 
The repository also contains tests related to Kubernetes Deployment and external plugins (kubernetai, metadata_edns0).

### Running Tests

The tests are run using CircleCI. New PRs in this repository will be tested against all the tests with CoreDNS built from master.

The configuration is set in such a way that they can be run in your fork, in case you want to run the tests in your fork before submitting a PR here.
CircleCI must be enabled on your fork for this to work.

### Adding and Testing New Tests, or Changes to Tests

The go tests are located in `/tests` directory tree. `/build` contains scripts for spinning up the test 
environment such as setting up a local Kubernetes cluster environment for Kubernetes related tests.
The configuration for running the tests is done in the .circleci/config.yaml file.

### Running Kubernetes Related CI Tests Locally

You can run these tests locally, though the process to get them working is not streamlined in any way.
At a high level, you should be able to do something like the following:
1. install/start a kubernetes cluster to run the tests.
2. make sure kubeconfig is set up to point to your cluster, and that kubectl works
3. create the required fixtures using `kubectl apply -f $GOPATH/src/github.com/coredns/ci/build/kubernetes/dns-test.yaml`. This creates a static set of test services/pods/namespaces (namespaces named test-1, test-2, etc).
4. build the docker image of coredns. `cd $GOPATH/src/github.com/coredns/coredns && make coredns SYSTEM="GOOS=linux" && docker build -t coredns .`
5. modify `$GOPATH/src/github.com/coredns/ci/build/coredns_deployment_patch.yaml` image to point to the local coredns docker image before patching the coredns deployment.
6. run the tests with `go test` .... e.g. `go test -v ./test/kubernetes/...`
