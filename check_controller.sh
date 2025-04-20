#!/bin/bash

echo "All pods in the cluster:"
kubectl get pods -A

echo "Checking controller pods:"
kubectl get pods -A | grep controller

echo "Checking Fio CRDs:"
kubectl get fios -A 