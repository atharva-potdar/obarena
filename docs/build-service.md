# Build Service

## Purpose

Consumes `submission.created` events from Redpanda. For each event, spawns an isolated build pod in the `builds` namespace that uses an init-container pipeline and pre-signed S3 URLs to download the source artifact, compile it, and upload the resulting binary back to SeaweedFS. Once the pod completes, the service publishes a lifecycle event with the result. Single responsibility — no HTTP endpoints, no authentication.

## Position in Pipeline

Second service in the pipeline. Consumes from the topic defined by `KAFKA_TOPIC` (default: `submission.lifecycle` — after submission-api) → produces to `submission.lifecycle` (consumed by sandbox-orchestrator).

## Event Contract

**Consumer group:** `build-service`

**Reads from:** `submission.lifecycle` (configurable via `KAFKA_TOPIC`)
**Writes to:** `submission.lifecycle` (configurable via `KAFKA_TOPIC`)

### Consumed: submission.created

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | `"submission.created"` |
| `submission_id` | string | UUID |
| `language` | string | `"cpp"`, `"rust"`, or `"go"` |
| `team_name` | string | Team display name |
| `artifact_path` | string | `"submissions/{submission_id}.tar.gz"` |
| `created_at` | int64 | Unix nanoseconds |

### Produced: build.complete

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | `"build.complete"` |
| `submission_id` | string | UUID |
| `binary_path` | string | `"builds/{submission_id}/binary"` |
| `language` | string | Language used |
| `team_name` | string | Team display name |
| `built_at` | int64 | Unix nanoseconds |

### Produced: build.failed

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | `"build.failed"` |
| `submission_id` | string | UUID |
| `reason` | string | Compiler error output, truncated to `MAX_LOG_BYTES` |
| `failed_at` | int64 | Unix nanoseconds |

## Operational Flow

1. Consume `submission.created` event from Redpanda.
2. Generate pre-signed GET URL for the source tarball and pre-signed PUT URL for the target binary in SeaweedFS. Note: The `s3.PresignClient` is created on-the-fly for each URL generation (no stored field).
3. Create a build pod in the `builds` namespace using a 3-init-container architecture:
   - **`download-source`**: Uses `wget` to pull the tarball via the GET URL.
   - **`build`**: Extracts the tarball and runs the language-specific compiler.
   - **`upload-binary`**: Uses `curl` to upload the compiled binary via the PUT URL.
4. Wait for the pod to complete (watching for `Succeeded` or `Failed` phase).
5. On success (pod reaches `Succeeded`):
   - Publish `build.complete` event.
6. On failure (pod reaches `Failed` or times out):
   - Fetch the pod logs from the `build` init container.
   - Publish `build.failed` event with truncated compiler stderr.
7. Delete build pod (always, regardless of outcome).
8. Commit consumer offsets.

## Endpoints

None. This service has no HTTP server.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `SEAWEEDFS_ENDPOINT` | `http://seaweedfs.platform.svc.cluster.local:8333` | SeaweedFS S3 endpoint |
| `REDPANDA_BROKERS` | `redpanda.platform.svc.cluster.local:9092` | Comma-separated broker list |
| `KAFKA_TOPIC` | `submission.lifecycle` | Topic for consumption and production |
| `MAX_LOG_BYTES` | `4096` | Max compiler output captured on failure |

## Dependencies

- SeaweedFS in `platform` namespace (S3-compatible storage)
- Redpanda in `platform` namespace
- Kubernetes API (in-cluster config)
- `github.com/twmb/franz-go` — Redpanda consumer/producer
- `github.com/aws/aws-sdk-go-v2` — S3 client
- `k8s.io/client-go` — Kubernetes client

## Build Toolchains

| Language | Image | Build Command |
|----------|-------|---------------|
| `cpp` | `gcc:16-trixie` | `g++ -static -O2 -o /workspace/binary /workspace/main.cpp` |
| `rust` | `rust:1.95-alpine` | `cd /workspace && RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --offline && cp $(find target/release -maxdepth 1 -type f -perm -111 ! -name '*.d' \| head -1) /workspace/binary` |
| `go` | `golang:1.26-alpine` | `cd /workspace && CGO_ENABLED=0 go build -mod=vendor -o /workspace/binary .` |

All binaries are statically linked for portability.

## Build Pod Spec

| Property | Value |
|----------|-------|
| Namespace | `builds` |
| InitContainers | `download-source` (`alpine:3.23`), `build` (Language-specific), `upload-binary` (`alpine/curl:8.9.1`) |
| Main Container | `done` (`alpine:3.23`, runs `true` to immediately transition to Succeeded) |
| Security Context | `runAsNonRoot: true`, `runAsUser: 65534`, `readOnlyRootFilesystem: true`, `drop: ALL`, `seccompProfile: RuntimeDefault`, `appArmorProfile: RuntimeDefault` |
| Volumes | EmptyDir at `/workspace` (512Mi), EmptyDir at `/tmp` (unlimited) |
| Resources (Build) | CPU: 1 request / 2 limit, Memory: 512Mi request / 2Gi limit |

## Constraints

- Build pods run in the `builds` namespace with a CiliumNetworkPolicy allowing egress *only* to SeaweedFS (ports 8333 and 8080) and DNS.
- No general internet access during build — all dependencies must be vendored.
- Build pod always deleted after completion (success or failure).
- Consumer group `build-service` ensures each submission is processed once.

## RBAC

Uses the `build-service` ServiceAccount (in `platform` namespace).
Bound to `build-pod-manager` Role in `builds` namespace:

- `pods`: create, get, list, watch, delete
- `pods/log`: get

## Helm Resources

| Property | Value |
|----------|-------|
| CPU request | 500m |
| CPU limit | 1000m |
| Memory request | 512Mi |
| Memory limit | 1024Mi |
| Autoscaling | KEDA Kafka (consumer group lag, max 4) |
