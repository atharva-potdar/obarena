#!/usr/bin/env bash
set -euo pipefail

# Ensure we have passwords defined
if [ -z "${TIMESCALEDB_PASSWORD:-}" ]; then
  echo "TIMESCALEDB_PASSWORD is not set. Defaulting to 'obarena' (NOT RECOMMENDED FOR PROD)"
  TIMESCALEDB_PASSWORD="obarena"
fi

if [ -z "${REDIS_PASSWORD:-}" ]; then
  echo "REDIS_PASSWORD is not set. Defaulting to 'redispass' (NOT RECOMMENDED FOR PROD)"
  REDIS_PASSWORD="redispass"
fi

echo "Updating helm dependencies..."
helm dependency update infra/helm/obarena-platform/ || true

echo "Applying Helm Chart..."
helm upgrade --install obarena-platform infra/helm/obarena-platform/ \
  --namespace platform \
  --create-namespace \
  -f infra/helm/obarena-platform/values-prod.yaml \
  --set timescaledb.password="$TIMESCALEDB_PASSWORD" \
  --set redis.password="$REDIS_PASSWORD" \
  --atomic \
  --timeout 10m

echo "Deployment successful."
# Extract ingress address
echo "Waiting for ingress IP..."
sleep 5
INGRESS_IP=$(kubectl get ingress obarena-ingress -n platform -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' || echo "Pending")
echo "Platform is reachable at: http://$INGRESS_IP"
