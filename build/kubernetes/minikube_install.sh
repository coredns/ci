#!/bin/bash
set -v

# Install minikube and kubectl
curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 && chmod +x minikube
curl -Lo kubectl  https://storage.googleapis.com/kubernetes-release/release/v1.10.0/bin/linux/amd64/kubectl && chmod +x kubectl
mv ./minikube /usr/local/bin/
mv ./kubectl /usr/local/bin/

# Start a local docker repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2
