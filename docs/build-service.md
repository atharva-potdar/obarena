# Build Service

## Overview

Consumes `submission.created` events from Redpanda. For each event, downloads
the source artifact from SeaweedFS, spawns an isolated build pod in the `builds`
namespace, compiles the code, uploads the resulting binary back to SeaweedFS,
and publishes a lifecycle event with the result.

Single responsibility — no HTTP endpoints, no auth, no sandboxing.
Runs as a single consumer in the `platform` namespace.

---

## Event Consumption

Topic:          submission.lifecycle
Consumer group: build-service
Events handled: submission.created

---

## Build Flow

1. Consume `submission.created` event
2. Download artifact from SeaweedFS at `event.artifact_path`
3. Spawn build pod in `builds` namespace:
   - Network egress denied (NetworkPolicy)
   - Source mounted via ConfigMap or init container from SeaweedFS
   - Language-specific image and build command
   - CPU and memory limits enforced
4. Wait for build pod to complete
5. On success:
   - Copy binary from pod to SeaweedFS at `builds/{submission_id}/binary`
   - Publish `build.complete` event
6. On failure:
   - Publish `build.failed` event with reason

---

## Build Images and Commands

| Language | Image                        | Build command                        |
|----------|------------------------------|--------------------------------------|
| cpp      | gcc:13-alpine                | g++ -O2 -o binary main.cpp           |
| rust     | rust:1.77-alpine             | rustc -O -o binary main.rs           |
| go       | golang:1.22-alpine           | go build -o binary .                 |

---

## Event Schema

Topic: submission.lifecycle
Key:   submission_id

build.complete
{
  "event":          "build.complete",
  "submission_id":  "uuid",
  "binary_path":    "builds/{submission_id}/binary",
  "language":       "cpp" | "rust" | "go",
  "team_name":      "string",
  "built_at":       1234567890    // unix nanoseconds
}

build.failed
{
  "event":          "build.failed",
  "submission_id":  "uuid",
  "reason":         "string",     // compiler error output, truncated to 4KB
  "failed_at":      1234567890
}

---

## Configuration

SEAWEEDFS_ENDPOINT    SeaweedFS S3 endpoint
                      default: <http://seaweedfs.platform.svc.cluster.local:8333>
REDPANDA_BROKERS      comma-separated broker list
                      default: redpanda.platform.svc.cluster.local:9092
BUILD_TIMEOUT_SECONDS max time to wait for a build pod to complete
                      default: 120
MAX_LOG_BYTES         max compiler output captured on failure
                      default: 4096

---

## Constraints

- Build pods run in the `builds` namespace with default-deny egress
- Build pods have no network access — cannot download dependencies
  Submissions must be self-contained single-file programs
- Binary size limit: 50MB
- Build timeout: 120 seconds (configurable)
- One build at a time per submission (no parallel builds for same ID)
- Build service uses the sandbox-orchestrator ServiceAccount for k8s API access
