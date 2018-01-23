# ci
Repository for continuous integration tests for the coredns project.  Currently only tests relating to kubernetes are contained here, but integration tests for other coredns plugins may be moved here or added here in the future. 

### Running Tests

You can initiate an integration test by including "/integration" in a comment for a PR in a linked repository.  The linked repositories are...

* `coredns/coredns`: Issuing an integration test from a PR in this repo will run the tests in `/coredns/ci/tests/kubernetes`.  These test a variety of coredns configurations in an attempt to provide wide integration level coverage for the `kubernetes` plugin. 
* `coredns/deployment`: Issuing an integration test from a PR in this repo will run the tests in `/coredns/ci/tests/k8sdeployment`.  This smaller set of tests validate the kubernetes deployment script creates a functional deployment.

After making the comment, the CoreDNS CI Bot should make a comment back in the originating PR that the request is received, and start the test.  The Bot will post a link to a log, and post results back to the same comment when the test is complete.  The tests typically take a few minutes to run.

The integration tests are executed one at a time, and no queue is implemented.  If a request is made while another test is in progress, it will wait for 5 minutes for the existing test to finish, or give up and post a comment to the originating PR.

You may request multiple integration tests for the same PR, but only one log is kept per PR.  A new integration request will overwrite the prior integration test log of the same PR.

### Adding and Testing New Tests, or Changes to Tests

The go tests are located in `/tests` directory tree. `/build` contains scripts for spinning up/down the test environment such as starting up minikube environment for kubernetes related tests.

When a test request is received, the `/coredns/ci` is cloned on the test server.  To test new tests, or changes to tests without merging first, you can specify a `/coredns/ci` PR number in the comment in the following way...

```
/integration-cipr22
```

The above will run integration tests using `/coredns/ci` PR 22.

