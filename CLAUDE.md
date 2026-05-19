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
