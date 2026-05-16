#!/bin/bash
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "==> Deploying platform via Helm"
helm upgrade --install obarena-platform infra/helm/obarena-platform/ \
  --namespace platform --create-namespace \
  --set image.tag=dev \
  --wait --timeout 300s

echo "==> infra-up complete"
