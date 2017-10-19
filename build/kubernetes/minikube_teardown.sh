#!/bin/bash

PR=$1

# stop local proxy
kill -9 `cat /var/run/kubectl_proxy.pid`

# delete minikube instance
minikube delete

# Delete the docker image repository
docker stop registry
docker rm registry