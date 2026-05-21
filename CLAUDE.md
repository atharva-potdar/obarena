# CLAUDE.md

This file provides guidance to AI assistants (like Claude) when working with the OBARENA platform codebase.

## 🏗️ Architecture Overview

The OBARENA platform is a distributed, event-driven system that evaluates contestant matching engines (orderbooks).

- **Core Paradigm**: Event-driven microservices communicating exclusively via Redpanda (Kafka). Direct internal HTTP calls between platform services are prohibited.
- **Language**: Go 1.26 monorepo located under `services/`.
- **Infrastructure**: Kubernetes (k0s), Cilium (L7 network policies), Longhorn (storage), Helm, Terraform.
- **Data Layers**: SeaweedFS (S3-compatible) for artifacts, TimescaleDB for telemetry, Redis for leaderboard state and pub/sub.

## 💻 Build & Deploy Commands

All infrastructure is managed via `just` (see `justfile` for all commands).

- **Full Local Dev Cycle**: `just` (builds images, loads to k0s, deploys Helm, runs smoke test)
- **Tear Down Local**: `just dev-teardown`
- **Clean Cache**: `just clean-cache`
- **Lint Helm Charts**: `just helm-lint`
- **Verify Go Code**: `go build -o /dev/null ./...` (Use this to verify changes don't break compilation)

## 🔧 Operational & Test Commands

**CRITICAL RULE**: Always use the `KUBECONFIG=~/.kube/config k0s kubectl` pattern when interacting with the cluster manually. Never use a bare `kubectl` command.

- **Watch Pod Status**: `watch -n 1 KUBECONFIG=~/.kube/config k0s kubectl get pods -A`
- **Check Build Logs**: `KUBECONFIG=~/.kube/config k0s kubectl logs -n platform deployment/build-service | tail -30`
- **Inspect Sandboxes**: `KUBECONFIG=~/.kube/config k0s kubectl describe pod <sandbox-pod-name> -n sandboxes`

- **Port Forwarding (Required for testing)**:
  - Submission API: `KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/submission-api 8080:8080`
  - SeaweedFS: `KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/seaweedfs 8333:8333`
  - Leaderboard UI: `KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/leaderboard-ws 8090:8090`

- **E2E Smoke Test Submission** (Requires 8080 & 8333 forwarded):

  ```bash
  RESP=$(curl -s -X POST http://localhost:8080/submissions -H "Content-Type: application/json" -d '{"language":"go","team_name":"test-team"}') && URL=$(echo $RESP | jq -r '.presigned_url') && ID=$(echo $RESP | jq -r '.submission_id') && curl -X PUT -T ~/Projects/testserver.tar.gz "$URL" --resolve "seaweedfs.platform.svc.cluster.local:8333:127.0.0.1" && curl -s -X POST http://localhost:8080/submissions/$ID/confirm
  ```

## 📝 Code Style & Conventions

- **Logging**: Use standard library structured logging (`slog`) exclusively.
- **Documentation & Comments**: Maintain a professional, clean standard. Do not write confusing walls of comments, but ensure complex algorithmic logic and package-level intent are clearly documented. Single-line comments are preferred for non-obvious logic. Anyone reading the codebase should easily understand the intent.
- **Commit Style**: Use semantic commits including the scope/domain (e.g., `fix(bot-runner): message` or `feat(submission-api): message`). The commit body must consist of an unordered list using hyphens (`-`) and newlines for a concise listing of changes.

## 🤖 Communication & Response Rules for AI

- **Tool Updates**: State in one concise sentence what you are about to do before starting tool calls.
- **Thinking Process**: State results, decisions, and outcomes directly. Do not narrate your internal deliberations.
- **Summaries**: Keep end-of-turn summaries extremely brief (one or two sentences), describing exactly what changed and what's next.
- **Format Match**: Match response styling and length to the complexity of the task; a simple question must receive a simple, direct answer.

## 🔐 Testing JWT Auth Locally

To test the Envoy JWT + rate limiting layer in dev, generate a keypair and convert to JWKS:

```bash
# 1. Generate RSA keypair (one-time)
openssl genrsa -out dev-jwt.key 2048
openssl rsa -in dev-jwt.key -pubout -out dev-jwt.pub

# 2. Convert PEM to JWKS (Envoy requires JWKS JSON, NOT raw PEM)
python3 scripts/pem-to-jwks.py dev-jwt.pub > dev-jwt.jwks

# 3. Deploy with JWT enabled
just infra-up \
  --set jwt.enabled=true \
  --set-file jwt.jwks=dev-jwt.jwks \
  --set jwt.issuer="dev-local"
```

> **Note:** After enabling `envoyConfig.enabled=true` in Cilium for the first time, the
> `ciliumclusterwideenvoyconfigs.cilium.io` CRD must exist. If the Cilium agent logs show
> `Still waiting for Cilium Operator to register CRDs`, apply it manually:
> ```bash
> KUBECONFIG=~/.kube/config k0s kubectl apply --server-side --force-conflicts \
>   -f https://raw.githubusercontent.com/cilium/cilium/v1.19.4/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwideenvoyconfigs.yaml
> ```

To generate a JWT token for a team `team-alpha`:

```bash
b64url() { base64 | tr -d '\n' | tr -d '=' | tr '/+' '_-'; }
HEADER=$(echo -n '{"alg":"RS256","typ":"JWT"}' | b64url)
PAYLOAD=$(echo -n '{"team_id":"team-alpha","iss":"dev-local","exp":1893456000}' | b64url)
SIGNATURE=$(echo -n "$HEADER.$PAYLOAD" | openssl dgst -sha256 -sign dev-jwt.key -binary | b64url)
export VALID_TOKEN="$HEADER.$PAYLOAD.$SIGNATURE"
```

To use this JWT token for a submission (run this from an in-cluster pod so it passes through the Cilium Envoy proxy, unlike port-forwarding which bypasses it):

```bash
# 1. Initialize the upload (this hits the Envoy JWT filter)
RESP=$(KUBECONFIG=~/.kube/config k0s kubectl run -i --rm test-client --image=curlimages/curl --restart=Never --quiet -- \
  curl -s -X POST http://submission-api.platform.svc.cluster.local:8080/submissions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $VALID_TOKEN" \
  -d '{"language":"go","team_name":"team-alpha"}') \
&& echo "Response: $RESP" \
&& URL=$(echo $RESP | jq -r '.presigned_url') \
&& ID=$(echo $RESP | jq -r '.submission_id')

# 2. Upload the payload directly to SeaweedFS (make sure port 8333 is forwarded)
echo "==> Uploading payload to SeaweedFS..."
curl -X PUT -T ~/Projects/testserver.tar.gz "$URL" --resolve "seaweedfs.platform.svc.cluster.local:8333:127.0.0.1"

# 3. Confirm the submission (hits the Envoy JWT filter again)
echo -e "\n==> Confirming submission $ID..."
KUBECONFIG=~/.kube/config k0s kubectl run -i --rm test-client --image=curlimages/curl --restart=Never --quiet -- \
  curl -s -X POST http://submission-api.platform.svc.cluster.local:8080/submissions/$ID/confirm \
  -H "Authorization: Bearer $VALID_TOKEN"
```
