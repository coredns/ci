test-coredns: fetch-coredns-pr build-docker start-k8s test-k8s

test-deployment: fetch-deployment-pr fetch-coredns start-k8s test-k8s-deployment

.PHONY: fetch-coredns-pr
fetch-coredns-pr:
	mkdir -p ${GOPATH}/src/${COREDNSPATH}
	cd ${GOPATH}/src/${COREDNSPATH} && \
	  git clone https://${COREDNSREPO}/coredns.git && \
	  cd coredns && \
	  git fetch origin +refs/pull/${PR}/merge:pr-${PR} && \
	  git checkout pr-${PR} && \
	  go get

.PHONY: fetch-coredns
fetch-coredns:
	mkdir -p ${GOPATH}/src/${COREDNSPATH}
	cd ${GOPATH}/src/${COREDNSPATH} && \
	  git clone https://${COREDNSREPO}/coredns.git && \
	  cd coredns && \
	  ${MAKE} godeps

.PHONY: fetch-deployment-pr
fetch-deployment-pr:
	mkdir -p ${GOPATH}/src/${COREDNSPATH}
	cd ${GOPATH}/src/${COREDNSPATH} && \
	  git clone https://${COREDNSREPO}/deployment.git && \
	  cd deployment && \
	  git fetch origin +refs/pull/${PR}/merge:pr-${PR} && \
	  git checkout pr-${PR}

.PHONY: build-docker
build-docker:
	# Start local docker image repo
	-docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2 || true

	# Build coredns docker image, and push to local repo
	cd ${GOPATH}/src/${COREDNSPATH}/coredns && \
	  ${MAKE} coredns SYSTEM="GOOS=linux" && \
	  docker build -t coredns . && \
	  docker tag coredns localhost:5000/coredns && \
	  docker push localhost:5000/coredns

.PHONY: start-k8s
start-k8s:
	# Set up minikube
	-sh ./build/kubernetes/minikube_setup.sh

.PHONY: test-k8s
test-k8s:
	# Integration tests
	go test -v ./test/kubernetes/...

.PHONY: test-k8s-deployment
test-k8s-deployment:
	# Integration tests
	go test -v ./test/k8sdeployment/...

.PHONY: clean-k8s
clean-k8s:
	# Clean up
	-sh ./build/kubernetes/minikube_teardown.sh

.PHONY: install-webhook
install-webhook:
	cp ./build/pr-comment-hook.sh /opt/bin/
	# For now, update /etc/webhook.conf and /etc/caddy/Caddyfile are manual

PHONY: install-minikube
install-minikube:
	# Install minikube
	sh ./build/kubernetes/minikube_install.sh

