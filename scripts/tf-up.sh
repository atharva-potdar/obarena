#!/usr/bin/env bash
set -euo pipefail

# End-to-end bootstrap script for Cloud Infrastructure

echo "Initializing Terraform..."
terraform -chdir=infra/terraform init

echo "Applying Terraform configuration..."
terraform -chdir=infra/terraform apply -auto-approve

# Retrieve outputs
REGISTRY_URL=$(terraform -chdir=infra/terraform output -raw registry_url)
KUBECONFIG_CMD=$(terraform -chdir=infra/terraform output -raw kubeconfig_command)

echo "Configuring kubectl..."
eval "$KUBECONFIG_CMD"

# Verify Cluster Connection
echo "Waiting for cluster to become ready..."
kubectl get nodes

echo "Installing metrics-server..."
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ || true
helm repo update metrics-server
helm upgrade --install metrics-server metrics-server/metrics-server \
  --namespace kube-system \
  --set args={--kubelet-insecure-tls} \
  --wait

echo "Installing Cluster Autoscaler..."
# We use the AWS EKS Cluster Autoscaler chart
helm repo add autoscaler https://kubernetes.github.io/autoscaler || true
helm repo update autoscaler
CLUSTER_NAME=$(terraform -chdir=infra/terraform output -raw cluster_name || echo "iicpc-platform")
REGION=$(terraform -chdir=infra/terraform output -raw region || echo "us-east-1")

helm upgrade --install cluster-autoscaler autoscaler/cluster-autoscaler \
  --namespace kube-system \
  --set autoDiscovery.clusterName="$CLUSTER_NAME" \
  --set awsRegion="$REGION" \
  --set extraArgs.scale-down-enabled=true \
  --set extraArgs.expander=least-waste \
  --set extraArgs.skip-nodes-with-system-pods=false \
  --wait

echo "Installing NGINX Ingress Controller..."
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx || true
helm repo update ingress-nginx
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --set controller.service.type=LoadBalancer \
  --set controller.ingressClassResource.name=nginx \
  --set controller.ingressClassResource.default=true \
  --wait

echo "Building and Pushing Docker images to ECR..."
bash scripts/push-images.sh

echo "Deploying the Helm Chart..."
bash scripts/helm-deploy.sh
