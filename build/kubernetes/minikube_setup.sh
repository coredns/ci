#!/bin/bash
set -v

ci_bin=$GOPATH/src/github.com/coredns/ci/build

# Start local docker image repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2

export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=$HOME
export CHANGE_MINIKUBE_NONE_USER=true
mkdir $HOME/.kube || true
touch $HOME/.kube/config

export KUBECONFIG=$HOME/.kube/config
if [[ -z ${K8S_VERSION} ]]; then
  minikube start --vm-driver=none
else
  minikube start --vm-driver=none --kubernetes-version=${K8S_VERSION}
fi

# Wait for kubernetes api service to be ready
for i in {1..60} # timeout for 2 minutes
do
   kubectl get po
   if [ $? -ne 1 ]; then
      break
  fi
  sleep 2
done

# Disable kube-dns in addon manager
minikube addons disable kube-dns

# Deploy test objects
kubectl create -f ${ci_bin}/kubernetes/dns-test.yaml

# Deploy coredns in place of kube-dns
kubectl apply -f ${ci_bin}/kubernetes/coredns.yaml

# Wait for coredns to be ready
for i in {1..60} # timeout for 2 minutes
do
  kubectl -n kube-system get pods | grep coredns
  kubectl -n kube-system get pods | grep coredns | grep Running && break
  sleep 2
done

# Wait for all test pods in test-1 to be ready (there are 5)
for i in {1..60} # timeout for 2 minutes
do
  if [ $(kubectl -n test-1 get pods | grep Running | wc -l) = "5" ]; then
    break
  fi
  sleep 2
done

# Give coredns a chance to load the pods/svcs into api cache
sleep 3

