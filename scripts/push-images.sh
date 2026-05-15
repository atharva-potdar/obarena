#!/usr/bin/env bash
set -euo pipefail

# Build and push service images to the ECR registry

REGISTRY=$(terraform -chdir=infra/terraform output -raw registry_url || true)
if [ -z "$REGISTRY" ]; then
    echo "Failed to get registry_url from terraform outputs. Did you run tf-up?"
    exit 1
fi

TAG=$(git rev-parse --short HEAD)

# Authenticate Docker to ECR
REGION=$(terraform -chdir=infra/terraform output -raw region || echo "us-east-1")
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

SERVICES=(
    "submission-api"
    "build-service"
    "sandbox-orchestrator"
    "bot-orchestrator"
    "bot-runner"
    "telemetry-ingester"
    "leaderboard-ws"
)

for svc in "${SERVICES[@]}"; do
    IMAGE_NAME="$REGISTRY/$svc:$TAG"
    echo "Building $IMAGE_NAME..."
    DOCKER_BUILDKIT=1 docker build -t "$IMAGE_NAME" -f "services/$svc/Dockerfile" .
    echo "Pushing $IMAGE_NAME..."
    docker push "$IMAGE_NAME"
done

echo "All images pushed with tag $TAG"
