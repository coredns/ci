#!/usr/bin/env bash
set -v

# Install kubectl
curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# Install kind
curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-$(uname)-amd64 && chmod +x ./kind && sudo mv ./kind /usr/local/bin/

# Create a single node cluster
kind create cluster --image kindest/node:${K8S_VERSION}

# Wait for cluster to be ready
JSONPATH='{range .items[*]}{@.metadata.name}:{range @.status.conditions[*]}{@.type}={@.status};{end}{end}'
out=''
until [[ "${out}" =~ 'Ready=True' ]]; do
  sleep 1
  out=$(kubectl get nodes -o jsonpath="$JSONPATH")
  echo "${out}"
done

# Scale the CoreDNS replicas to simplify testing
kubectl scale -n kube-system deployment/coredns --replicas=1

# Patch CoreDNS deployment to use local coredns image
kubectl patch deployment coredns -n kube-system -p "$(cat ~/go/src/${CIRCLE_PROJECT_USERNAME}/ci/build/kubernetes/coredns_deployment_patch.yaml)"

# Patch CoreDNS clusterRoles to allow list/watch of EndpointSlice.
# Remove this once EndpointSlice is part of the default CoreDNS clusterRoles.
kubectl patch clusterroles system:coredns -n kube-system -p "$(cat ~/go/src/${CIRCLE_PROJECT_USERNAME}/ci/build/kubernetes/coredns_clusterroles_patch.yaml)"

# Deploy test objects
kubectl create -f ~/go/src/${CIRCLE_PROJECT_USERNAME}/ci/build/kubernetes/dns-test.yaml
