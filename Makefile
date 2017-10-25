test: fetch-pr deploy-k8s test-k8s

.PHONY: fetch-pr
fetch-pr:

	# Get coredns code
	mkdir -p ${GOPATH}/src/${COREDNSPATH}
	cd ${GOPATH}/src/${COREDNSPATH} && \
	  git clone https://${COREDNSREPO}/coredns.git && \
	  cd coredns && \
	  git fetch --depth 1 origin pull/${PR}/head:pr-${PR} && \
	  git checkout pr-${PR}

.PHONY: deploy-k8s
deploy-k8s:
	# Start local docker image repo (k8s must pull images from a repo)
	-docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2 || true

	# Build coredns docker image, and push to local repo
	cd ${GOPATH}/src/${COREDNSPATH}/coredns && \
	  ${MAKE} coredns SYSTEM="GOOS=linux" && \
	  docker build -t coredns . && \
	  docker tag coredns localhost:5000/coredns && \
	  docker push localhost:5000/coredns

	# Set up minikube
	-sh ./build/kubernetes/minikube_setup.sh

.PHONY: test-k8s
test-k8s:
	# Do tests
	go test -v ./test/kubernetes/...

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

