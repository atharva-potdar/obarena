#!/usr/bin/env bash
set -euo pipefail

# End-to-end bootstrap script for Cloud Infrastructure (k0s on EC2)

echo "Initializing Terraform..."
terraform -chdir=infra/terraform init

echo "Applying Terraform configuration..."
terraform -chdir=infra/terraform apply -auto-approve

echo "Generating Ansible inventory from Terraform outputs..."
bash scripts/generate-inventory.sh

echo "Running Ansible playbook for prod..."
ansible-playbook site.yml -i inventory-prod.ini -e env=prod

echo "Building and pushing Docker images to ECR..."
bash scripts/push-images.sh

echo "Deploying the Helm chart..."
bash scripts/helm-deploy.sh
