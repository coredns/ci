#!/bin/bash
set -v

# Setup Kubectl
curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# Setup Minikube
mkdir -p ${HOME}/.kube
touch ${HOME}/.kube/config
curl -Lo minikube https://github.com/kubernetes/minikube/releases/download/${MINIKUBE_VERSION}/minikube-linux-amd64 && chmod +x minikube && sudo mv minikube /usr/local/bin/

# Start a local docker repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2

# Start Minikube
sudo -E minikube start --vm-driver=none --cpus 2 --memory 2048 --kubernetes-version=${K8S_VERSION}

# Wait for minikube to setup
JSONPATH='{range .items[*]}{@.metadata.name}:{range @.status.conditions[*]}{@.type}={@.status};{end}{end}';
until kubectl get nodes -o jsonpath="$JSONPATH" 2>&1 | grep -q "Ready=True"; do
  sleep 1;
done

# Scale the CoreDNS replicas to 1 for more accuracy.
kubectl scale -n kube-system deployment/coredns --replicas=1

# Patch CoreDNS to update deployment.
kubectl patch deployment coredns -n kube-system -p "$(cat ~/go/src/${CIRCLE_PROJECT_USERNAME}/ci/build/kubernetes/coredns_deployment_patch.yaml)"

# Deploy test objects
kubectl create -f ~/go/src/${CIRCLE_PROJECT_USERNAME}/ci/build/kubernetes/dns-test.yaml

# Add federation labels to node
kubectl label nodes minikube failure-domain.beta.kubernetes.io/zone=fdzone
kubectl label nodes minikube failure-domain.beta.kubernetes.io/region=fdregion

# Start local proxy (for out-of-cluster tests)
kubectl proxy --port=8080 2> /dev/null &
echo -n $! > sudo /var/run/kubectl_proxy.pid
sleep 3
