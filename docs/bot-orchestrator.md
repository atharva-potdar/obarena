# Bot Orchestrator

## Overview

Consumes `sandbox.ready` events from Redpanda. For each event, spawns a bot
runner Job in the `bots` namespace pointed at the sandbox pod's IP and ports.
Waits for the Job to complete, then deletes the sandbox pod and publishes a
lifecycle event with the result.

Single responsibility — no HTTP endpoints, no build logic, no scoring.
Runs as a single consumer in the `platform` namespace.

---

## Event Consumption

Topic:          submission.lifecycle
Consumer group: bot-orchestrator
Events handled: sandbox.ready

---

## Orchestration Flow

1. Consume `sandbox.ready` event
2. Spawn bot runner Job in `bots` namespace:
   - Image: bot-runner:dev
   - Target endpoint from event: `ws://{pod_ip}:{ws_port}/stream`
   - Test run ID: submission_id
   - Configured via env vars
3. Wait for Job to complete (succeeded or failed)
4. Collect Job logs
5. Delete sandbox pod from `sandboxes` namespace
6. Delete bot runner Job from `bots` namespace
7. Publish `test.complete` event with result

---

## Bot Runner Job Spec

Image:          bot-runner:dev
Restart policy: Never
Parallelism:    1
Completions:    1

Environment:
  TARGET_ENDPOINT       ws://{pod_ip}:{ws_port}/stream
  NUM_BOTS              50
  DURATION_SECONDS      60
  TEST_RUN_ID           {submission_id}
  REDPANDA_BROKERS      {redpanda_brokers}

Resource Limits:
  CPU request:    500m
  CPU limit:      2
  Memory request: 256Mi
  Memory limit:   512Mi

---

## Event Schema

Topic: submission.lifecycle
Key:   submission_id

test.complete
{
  "event":          "test.complete",
  "submission_id":  "uuid",
  "team_name":      "string",
  "success":        true | false,
  "reason":         "string",     // empty on success, error on failure
  "completed_at":   1234567890    // unix nanoseconds
}

---

## Configuration

REDPANDA_BROKERS        comma-separated broker list
                        default: redpanda.platform.svc.cluster.local:9092
NUM_BOTS                number of concurrent bot goroutines per runner
                        default: 50
DURATION_SECONDS        how long to run the bot fleet
                        default: 60
JOB_TIMEOUT_SECONDS     max time to wait for bot runner Job to complete
                        default: 120

---

## RBAC

Uses the `sandbox-orchestrator` ServiceAccount (platform namespace).
Needs additional permissions:

In `bots` namespace:
- jobs:     create, get, list, watch, delete
- pods:     get, list, watch
- pods/log: get

In `sandboxes` namespace:
- pods: delete (already granted via sandbox-pod-manager Role)

---

## Constraints

- One bot runner Job per sandbox, run once then tear down
- Sandbox pod is always deleted after test, regardless of outcome
- Bot runner Job is always deleted after completion, regardless of outcome
- Bot orchestrator uses the sandbox-orchestrator ServiceAccount for k8s API access
- NUM_BOTS and DURATION_SECONDS are platform-controlled — contestants cannot influence them
