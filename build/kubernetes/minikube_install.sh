#!/bin/bash
set -v

# Install minikube and kubectl
curl -Lo minikube https://storage.googleapis.com/minikube/releases/v0.33.1/minikube-linux-amd64 && chmod +x minikube
curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl
mv ./minikube /usr/local/bin/
mv ./kubectl /usr/local/bin/

# Start a local docker repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2
