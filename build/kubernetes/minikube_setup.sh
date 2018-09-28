#!/bin/bash
set -v

ci_bin=$GOPATH/src/github.com/coredns/ci/build

# Start local docker image repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2

# Start minikube
export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=$HOME
export CHANGE_MINIKUBE_NONE_USER=true
mkdir $HOME/.kube || true
touch $HOME/.kube/config

export KUBECONFIG=$HOME/.kube/config

minikube start --vm-driver=none --kubernetes-version=v1.10.0

# Wait for minkube's api service to be ready
for i in {1..60} # timeout for 2 minutes
do
   kubectl get po
   if [ $? -ne 1 ]; then
      break
  fi
  sleep 2
done

# Disable kube-dns in addon manager
minikube addons disable kube-dns 2> /dev/null
kubectl delete deployment kube-dns -n kube-system

# Deploy test objects
kubectl create -f ${ci_bin}/kubernetes/dns-test.yaml

# Wait for pods in test-1 namespace to be Running
for i in {1..60} # timeout after 1 minute
do
  readypods=`kubectl get po -n test-1 | grep Running | wc -l`
  test
  if [ 3 -eq ${readypods} ]; then
    break
  fi
  sleep 1
done

if [ 3 -ne ${readypods} ]; then
  echo "Timed out waiting for 3 pods in test-1. Saw $readypods."
  kubectl get po -n test-1
  exit 1
fi

# Deploy coredns in place of kube-dns
kubectl apply -f ${ci_bin}/kubernetes/coredns.yaml

# Start local proxy (for out-of-cluster tests)
kubectl proxy --port=8080 2> /dev/null &
echo -n $! > /var/run/kubectl_proxy.pid
sleep 3

