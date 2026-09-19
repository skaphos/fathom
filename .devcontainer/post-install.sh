#!/bin/bash
set -euxo pipefail

KIND_VERSION=v0.33.0
KIND_SHA256=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
KUBEBUILDER_VERSION=v4.16.0
KUBEBUILDER_SHA256=539984e58832044f2326b1bb940c1eaf784f1849e65d06d3a4a4a8b5dfac7ca7
KUBECTL_VERSION=v1.37.0
KUBECTL_SHA256=6129359f4e1f3848a5572ccb0b26cf28b8ca08cef38c95a765b2f64a2c961a2f

curl -fsSLo ./kind "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-linux-amd64"
echo "${KIND_SHA256}  kind" | sha256sum --check
chmod +x ./kind
mv ./kind /usr/local/bin/kind

curl -fsSLo kubebuilder "https://github.com/kubernetes-sigs/kubebuilder/releases/download/${KUBEBUILDER_VERSION}/kubebuilder_linux_amd64"
echo "${KUBEBUILDER_SHA256}  kubebuilder" | sha256sum --check
chmod +x kubebuilder
mv kubebuilder /usr/local/bin/

curl -fsSLO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
echo "${KUBECTL_SHA256}  kubectl" | sha256sum --check
chmod +x kubectl
mv kubectl /usr/local/bin/kubectl

if ! docker network inspect kind >/dev/null 2>&1; then
  docker network create -d=bridge --subnet=172.19.0.0/24 kind
fi

kind version
kubebuilder version
docker --version
go version
kubectl version --client
