# Architecture

System-level overview of the OBARENA orderbook engine evaluation platform.

## Event Flow

```
contestant → submission-api (init) → SeaweedFS (S3 upload) → submission-api (confirm) → [Redpanda: submission.lifecycle] → build-service
build-service → [Redpanda: submission.lifecycle] → sandbox-orchestrator
sandbox-orchestrator → [Redpanda: submission.lifecycle] → bot-orchestrator
bot-orchestrator → K8s Job → bot-runner → [Redpanda: bot.metrics] → telemetry-ingester
telemetry-ingester → Redis ZADD → leaderboard-ws → frontend (WebSocket)
```

```mermaid
flowchart LR
    A[Contestant] -->|1. POST /submissions (init)| B[submission-api]
    B -->|2. presigned_url| A
    A -->|3. S3 PUT source| M[(SeaweedFS\nsubmissions bucket)]
    A -->|4. POST /submissions/:id/confirm| B
    B -->|5. submission.created| C[(Redpanda\nsubmission.lifecycle)]
    C -->|consume| D[build-service]
    D -->|build.complete| C
    C -->|consume| E[sandbox-orchestrator]
    E -->|sandbox.ready| C
    C -->|consume| F[bot-orchestrator]
    F -->|creates| G[K8s Job: bot-runner]
    G -->|bot.metrics| H[(Redpanda\nbot.metrics)]
    H -->|consume| I[telemetry-ingester]
    I -->|ZADD + PUBLISH| J[(Redis)]
    J -->|read + subscribe| K[leaderboard-ws]
    K -->|WebSocket| L[Browser Frontend]

    D -->|S3 GET| M
    D -->|S3 PUT| M
    E -->|S3 GET| M
    I -->|batch insert| O[(TimescaleDB)]
```

## Services

| Service | Namespace | Type | Replicas | Purpose |
|---------|-----------|------|----------|---------|
| submission-api | platform | Deployment | 2–10 (HPA) | Accept code submissions |
| build-service | platform | Deployment | 1–4 (KEDA) | Compile submitted code |
| sandbox-orchestrator | platform | Deployment | 1–4 (KEDA) | Deploy binaries as sandbox pods |
| bot-orchestrator | platform | Deployment | 1 (singleton) | Run bot test fleet |
| telemetry-ingester | platform | Deployment | 1–8 (KEDA) | Score and store results |
| leaderboard-ws | platform | Deployment | 2–6 (HPA) | Serve live leaderboard |
| bot-runner | bots | Job | 1 per test | Execute correctness + load tests |
| sandbox | sandboxes | Pod | 1 per submission | Run contestant binary |
| build | builds | Pod | 1 per submission | Compile contestant code |

## Topic Topology

| Service | Reads | Writes |
|---------|-------|--------|
| submission-api | — | `submission.lifecycle` (`submission.created`) |
| build-service | `submission.lifecycle` (group: `build-service`) | `submission.lifecycle` (`build.complete`, `build.failed`) |
| sandbox-orchestrator | `submission.lifecycle` (group: `sandbox-orchestrator`) | `submission.lifecycle` (`sandbox.ready`, `sandbox.failed`) |
| bot-orchestrator | `submission.lifecycle` (group: `bot-orchestrator`) | `submission.lifecycle` (`test.complete`) |
| bot-runner | — | `bot.metrics` (`bot.metrics`) |
| telemetry-ingester | `bot.metrics` (group: `telemetry-ingester`) | — |
| leaderboard-ws | — | — |

## Namespace Isolation Model

Network isolation is enforced using `CiliumNetworkPolicy` resources (L7-aware), rather than vanilla Kubernetes `NetworkPolicy`.

| Namespace | Contents | Network Policy |
|-----------|----------|----------------|
| `platform` | All services, Redpanda, Redis, TimescaleDB, SeaweedFS | Default deny egress; explicit allow for DNS, K8s API, Redpanda, Redis, TimescaleDB, SeaweedFS, and all other namespaces |
| `builds` | Build pods (ephemeral) | Default deny egress; egress allowed *only* to SeaweedFS (ports 8333, 8080) and DNS |
| `sandboxes` | Sandbox pods (ephemeral) | Default deny egress; ingress from `bots` only; egress to SeaweedFS (ports 8333, 8080) and DNS only |
| `bots` | Bot runner Jobs (ephemeral) | Default deny egress; egress to `sandboxes`, `platform` (Redpanda only), and DNS |

### Cross-Namespace Communication

```
platform services → builds namespace    : K8s API (create/manage build pods)
platform services → sandboxes namespace : K8s API (create/manage sandbox pods)
platform services → bots namespace      : K8s API (create/manage bot Jobs)
bots namespace → sandboxes namespace    : WebSocket (load test traffic)
bots namespace → platform namespace     : Redpanda (publish metrics)
sandboxes namespace → platform namespace: SeaweedFS (binary download via InitContainer)
builds namespace → platform namespace   : SeaweedFS (source download and binary upload via InitContainers)
```

## Security Boundaries

### Sandbox Pod Security

Every sandbox pod (running contestant code) enforces:

| Control | Configuration |
|---------|--------------|
| User | `runAsUser: 65534` (nobody), `runAsNonRoot: true` |
| Privilege escalation | `allowPrivilegeEscalation: false` |
| Filesystem | `readOnlyRootFilesystem: true` |
| Capabilities | `drop: ["ALL"]` |
| Seccomp | `type: Localhost`, profile: `sandbox-seccomp.json` |
| AppArmor | `type: RuntimeDefault` |
| Service account | `automountServiceAccountToken: false` |
| Disk | EmptyDir with 256Mi limit at `/sandbox`, unlimited at `/tmp` |

### Build Pod Security

Build pods operate similarly to sandboxes:

| Control | Configuration |
|---------|--------------|
| User | `runAsUser: 65534` (nobody), `runAsNonRoot: true` |
| Privilege escalation | `allowPrivilegeEscalation: false` |
| Filesystem | `readOnlyRootFilesystem: true` |
| Capabilities | `drop: ["ALL"]` |
| Seccomp | `type: RuntimeDefault` |
| AppArmor | `type: RuntimeDefault` |
| Workspace | EmptyDir with 512Mi limit |
| Restart | `Never` |

### Bot Runner Job Security

| Control | Configuration |
|---------|--------------|
| Restart | `Never` |
| Backoff limit | 0 (no retries) |

## RBAC

| ServiceAccount | Namespace | Role | Scope | Permissions |
|---------------|-----------|------|-------|-------------|
| `sandbox-orchestrator` | platform | `sandbox-pod-manager` | `sandboxes` | pods: create/get/list/watch/delete, pods/log: get |
| `build-service` | platform | `build-pod-manager` | `builds` | pods: create/get/list/watch/delete, pods/log: get |
| `bot-orchestrator` | platform | `bot-job-manager` | `bots` | jobs: create/watch/delete |
| `bot-orchestrator` | platform | `sandbox-pod-manager` | `sandboxes` | pods: create/get/list/watch/delete, pods/log: get (sandbox teardown after test) |

## Data Flow

```
Source Code (tar.gz)
  → client requests pre-signed URL from submission-api and uploads directly to SeaweedFS (submissions bucket)
  → build-service downloads from SeaweedFS via presigned URL (download-source InitContainer)
  → build-service compiles in isolated pod
  → build-service uploads binary to SeaweedFS via presigned URL (upload-binary InitContainer)
  → sandbox-orchestrator downloads binary via presigned URL (InitContainer)
  → sandbox pod executes binary

Metrics
  → bot-runner measures latency (hdrhistogram) and throughput
  → bot-runner publishes to Redpanda (bot.metrics topic)
  → telemetry-ingester consumes and computes composite score
  → telemetry-ingester writes to TimescaleDB (historical)
  → telemetry-ingester writes to Redis (live leaderboard)
  → leaderboard-ws reads Redis and pushes to frontend
```

## Infrastructure

| Component | Image | Purpose |
|-----------|-------|---------|
| Redpanda | `redpandadata/redpanda:v26.1.7` | Event streaming (Kafka-compatible) |
| Redis | `redis:8-alpine` | Live leaderboard + pub/sub |
| TimescaleDB | `timescale/timescaledb:latest-pg18` | Historical telemetry storage |
| SeaweedFS | `chrislusf/seaweedfs:latest` | S3-compatible object storage |

### Storage

| Volume | Size | Purpose |
|--------|------|---------|
| SeaweedFS | 10Gi | PVC mapped to the SeaweedFS volume server. All source artifacts and compiled binaries are stored here. |
| TimescaleDB | 10Gi | PVC mapped to TimescaleDB. Telemetry events and submission scores are stored here. |
