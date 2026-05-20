# Sandbox Network Security & Hot-Path Integrity

## The Philosophy: Security vs. Physics

Applying Layer 7 (L7) network policies blindly introduces user-space context switching (Envoy proxy overhead) into the network path. For a microsecond-deterministic platform, we must strictly separate the **Cold Path** (where L7 proxy latency is acceptable for security) from the **Hot Path** (where bare-metal L4 speed is mandatory).

Untrusted sandbox pods operate under a **Default-Deny** egress posture with only three permitted paths. Stateful databases (TimescaleDB, Redis) are completely airgapped from the sandbox and can only be reached downstream via the `telemetry-ingester`.

## Egress Rules Breakdown

### 1. DNS (CoreDNS) — The Cold Path

| Property | Value |
|---|---|
| Protocol | UDP/TCP 53 |
| Enforcement | Layer 7 (Cilium DNS Parser) |

**Why:** A malicious bot could use DNS tunneling to exfiltrate proprietary platform code or establish a reverse shell. By enforcing L7 DNS inspection, Cilium drops malformed UDP packets and only allows RFC-compliant DNS queries.

**Impact:** A microsecond delay during pod startup. Zero impact on the benchmark.

### 2. SeaweedFS (Storage) — The Cold Path

| Property | Value |
|---|---|
| Protocol | TCP 8333 (filer), TCP 8080 (volume server) |
| Enforcement | Layer 7 (Cilium HTTP Parser, GET only) |

**Why:** The sandbox `InitContainer` only needs to download the bot binary. It has no legitimate reason to upload or delete files. The L7 policy strictly enforces `method: "GET"`, silently dropping any `PUT` or `DELETE` requests. Port 8080 is included because the SeaweedFS filer on 8333 redirects GET requests to the volume server on 8080.

**Impact:** Evaluated only during the InitContainer phase before the benchmark clock starts. Zero impact on the hot path.

### 3. Redpanda (Event Bus) — The Hot/Execution Path

| Property | Value |
|---|---|
| Protocol | TCP 9092 |
| Enforcement | Layer 4 (eBPF TCP Routing) + Broker-Side ACLs |

**Why:** We explicitly **drop L7 HTTP/Kafka inspection** here. Forcing Envoy to parse Kafka wire protocol payloads introduces severe latency jitter. Instead, we enforce security at the broker level using Redpanda's native SASL/SCRAM authentication. The bot is injected with credentials for a `sandbox_bot` user, and Redpanda's in-memory ACLs restrict that user strictly to `Write` operations on the `bot.metrics` topic.

**Impact:** Pure eBPF socket-level redirection (O(1) routing). Zero Envoy proxy overhead. Maximum hardware speed.

---

## Submission API: JWT Auth + Per-Team Rate Limiting

Inbound traffic from `world` to `submission-api:8080` passes through a `CiliumEnvoyConfig` that runs two HTTP filters inside Cilium's embedded Envoy proxy (no sidecar injection):

### Filter 1: `envoy.filters.http.jwt_authn`

- Validates `Authorization: Bearer <token>` using an offline RSA public key (no JWKS fetch on the hot path).
- Extracts the `team_id` claim directly into the `x-team-id` request header via `claim_to_headers`.
- Strips the `Authorization` header before forwarding upstream — internal services never receive raw JWT tokens.
- Rejects requests without a valid token with `401 Unauthorized`.

### Filter 2: `envoy.filters.http.local_ratelimit`

- Token bucket: 10 requests / 60 seconds per team (configurable via Helm values).
- Uses a **wildcard descriptor** (key `team_id`, no static value) so Envoy dynamically creates one independent token bucket per unique `team_id` value. One team spamming the API cannot exhaust capacity for other teams.
- LRU cache capped at 200 teams (`max_dynamic_descriptors`). Per-node, in-memory — no Redis required.
- Returns standard `X-RateLimit-*` response headers (RFC draft v3).

### Why No Global Redis?

Teams are submitting 50MB files occasionally, not streaming high-frequency API calls. Per-node token-bucket counters provide independent rate limiting on each node. The tradeoff (slightly higher effective rate limit in multi-node prod due to per-node buckets) is acceptable for this use case.

---

## CiliumNetworkPolicy Reference

```yaml
apiVersion: "cilium.io/v2"
kind: CiliumNetworkPolicy
metadata:
  name: sandboxes-policy
  namespace: sandboxes
spec:
  endpointSelector:
    matchLabels:
      app.kubernetes.io/name: sandbox
  egress:
    # 1. L7 DNS: Prevent DNS Tunneling (Cold Path)
    - toEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: kube-system
            k8s-app: kube-dns
      toPorts:
        - ports:
            - port: "53"
              protocol: UDP
            - port: "53"
              protocol: TCP
          rules:
            dns:
              - matchPattern: "*"

    # 2. L7 HTTP: GET-only to SeaweedFS (Cold Path)
    - toEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: platform
            app.kubernetes.io/name: seaweedfs
      toPorts:
        - ports:
            - port: "8333"
              protocol: TCP
            - port: "8080"
              protocol: TCP
          rules:
            http:
              - method: "GET"

    # 3. L4 TCP: Redpanda (Hot Path — pure eBPF, security via broker ACLs)
    - toEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: platform
            app.kubernetes.io/name: redpanda
      toPorts:
        - ports:
            - port: "9092"
              protocol: TCP
```
