#!/usr/bin/env bash
set -euo pipefail

echo "==> Checking k3s installation..."
if ! command -v k3s &>/dev/null; then
  curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--write-kubeconfig-mode 644" sh -
else
  echo "k3s is already installed."
fi

rc="$HOME/.$(basename "$SHELL")rc"
echo "==> Configuring KUBECONFIG..."
if ! grep -q "KUBECONFIG=/etc/rancher/k3s/k3s.yaml" "$rc"; then
  echo 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml' >>"$rc"
fi
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

echo "==> Checking Helm installation..."
if ! command -v helm &>/dev/null; then
  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4 | bash
else
  echo "Helm is already installed."
fi

echo "==> Checking Just installation..."
if ! command -v just &>/dev/null; then
  curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | sudo bash -s -- --to /usr/bin
else
  echo "Just is already installed."
fi

echo "==> Host setup complete!"
