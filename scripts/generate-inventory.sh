#!/usr/bin/env bash
set -euo pipefail

# Generate inventory-prod.ini from Terraform outputs.
# Requires: jq, terraform

CONTROLLER_IP=$(terraform -chdir=infra/terraform output -raw controller_ip)
PLATFORM_IPS=$(terraform -chdir=infra/terraform output -json platform_ips | jq -r '.[]')
SANDBOX_IPS=$(terraform -chdir=infra/terraform output -json sandbox_ips | jq -r '.[]')

cat > inventory-prod.ini <<EOF
[k0s_controller]
${CONTROLLER_IP} ansible_user=ubuntu ansible_ssh_private_key_file=~/.ssh/k0s-key.pem

[k0s_workers]
$(echo "$PLATFORM_IPS" | while read -r ip; do
  echo "${ip} ansible_user=ubuntu ansible_ssh_private_key_file=~/.ssh/k0s-key.pem"
done)

[sandbox]
$(echo "$SANDBOX_IPS" | while read -r ip; do
  echo "${ip} ansible_user=ubuntu ansible_ssh_private_key_file=~/.ssh/k0s-key.pem"
done)

[k0s_all_workers:children]
k0s_workers
sandbox
EOF

echo "==> Generated inventory-prod.ini with:"
echo "    Controller: ${CONTROLLER_IP}"
echo "    Platform workers: $(echo "$PLATFORM_IPS" | wc -l)"
echo "    Sandbox workers: $(echo "$SANDBOX_IPS" | wc -l)"
