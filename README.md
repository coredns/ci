# ci
Repository for continuous integration tests for the coredns project.  Currently only tests relating to kubernetes are contained here, but integration tests for other coredns plugins may be moved here or added here in the future. 

### Running Tests

You can initiate an integration test by including "/integration" in a comment for a PR in a linked repository.  The linked repositories are...

* `coredns/coredns`: Issuing an integration test from a PR in this repo will run the tests in `/coredns/ci/tests/kubernetes`.  These test a variety of coredns configurations in an attempt to provide wide integration level coverage for the `kubernetes` plugin. 
* `coredns/deployment`: Issuing an integration test from a PR in this repo will run the tests in `/coredns/ci/tests/k8sdeployment`.  This smaller set of tests validate the kubernetes deployment script creates a functional deployment.

After making the comment, the CoreDNS CI Bot should post a status back in the originating PR that the request is received, and start the test.  The tests typically take a 3-5 minutes to run.

The integration tests are executed one at a time, and no queue is implemented.  If a request is made while another test is in progress, it will wait for 5 minutes for the existing test to finish, or give up and update with an error the status to the originating PR.

You may request multiple integration tests for the same PR, but only one log is kept per commit.  A new integration request will overwrite the prior integration test log of the same commit.

### Adding and Testing New Tests, or Changes to Tests

The go tests are located in `/tests` directory tree. `/build` contains scripts for spinning up/down the test environment such as starting up minikube environment for kubernetes related tests.

When a test request is received, the `/coredns/ci` is cloned on the test server.  To test new tests, or changes to tests without merging first, you can specify a `/coredns/ci` PR number in the comment in the following way...

```
/integration-cipr22
```

The above will run integration tests using `/coredns/ci` PR 22.

### Running Kubernetes Related CI Tests Locally

You can run these tests locally, though the process to get them working is not streamlined in any way.
At a high level, you should be able to do something like the following:
1. install/start minikube - if you already use minikube for other things, you may opt to create a separate profile.
2. make sure kubeconfig is set up to point to minikube, and that kubectl works
3. create the required fixtures using `kubectl apply -f $GOPATH/src/github.com/coredns/ci/build/dns-test.yaml`. This creates a static set of test services/pods/namespaces (namespaces named test-1, test-2, etc).
4. build the docker image of coredns. `cd $GOPATH/src/github.com/coredns/coredns && make coredns SYSTEM="GOOS=linux" && docker build -t coredns .`
5. modify `$GOPATH/src/github.com/coredns/ci/build/coredns_deployment_patch.yaml` image to point to the local coredns docker image before patching the coredns deployment.
6. run the tests with `go test` .... e.g. `go test -v ./test/kubernetes/...`
