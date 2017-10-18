#!/bin/bash

PR=$1

# delete minikube instance
minikube delete

# Delete the docker image repository
docker stop registry
docker rm registry