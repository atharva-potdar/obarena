# Infrastructure Overview

The OBARENA platform relies on a combination of Terraform, Ansible, and Helm to manage infrastructure. This document outlines the Kubernetes-level architecture and configuration.

## Namespace Isolation Model

The cluster is strictly segmented into four namespaces:

1. **`kube-system`**: Core Kubernetes services, Cilium, k0s controllers.
2. **`platform`**: Long-running infrastructure and orchestration services (submission-api, build-service, orchestrators, Redpanda, SeaweedFS, TimescaleDB, Redis).
3. **`builds`**: Ephemeral compilation pods. Isolated to prevent unauthorized network access during untrusted builds.
4. **`sandboxes`**: Ephemeral, strictly confined pods running contestant orderbooks.
5. **`bots`**: Ephemeral Kubernetes Jobs (`bot-runner`) that act as load generators and correctness validators.

## Helm Chart & Values Hierarchy

The platform is deployed via a single monolithic Helm chart located at `infra/helm/obarena-platform/`.

Configuration is managed hierarchically:
- **`values.yaml`**: The base source of truth. Contains all default configurations, image tags, and base resource requests. Works out-of-the-box for local development.
- **`values-dev.yaml`**: Overrides specifically for extended development or staging environments.
- **`values-prod.yaml`**: Production overrides. Enforces strict node selectors, tolerations, high availability (replica counts > 1), and production-grade resource limits and requests.

When deploying, the appropriate values file is passed:
```bash
helm upgrade --install obarena infra/helm/obarena-platform -f infra/helm/obarena-platform/values-prod.yaml
```

## Storage & Longhorn

Persistent state (TimescaleDB data, Redpanda logs, SeaweedFS volumes) relies on a default Kubernetes `StorageClass`.

- **Local Dev**: The Ansible playbook automatically installs **Longhorn** and sets it as the default storage class. It provisions storage dynamically using the node's filesystem.
- **Production (AWS)**: Terraform provisions the AWS EBS CSI driver, which provides `gp3` volumes dynamically.

StatefulSets request storage via standard `PersistentVolumeClaims` (PVCs).

## Network Policies (Cilium)

We use `CiliumNetworkPolicy` (L7-aware) rather than vanilla Kubernetes `NetworkPolicy`. 

- **Default Deny**: The `platform`, `builds`, `sandboxes`, and `bots` namespaces all begin with a default-deny ingress and egress rule.
- **Allow-listing**: Traffic is explicitly permitted.
  - Inter-service communication is primarily restricted to talking to Redpanda (`9092`).
  - `builds` and `sandboxes` are permitted egress only to SeaweedFS (for artifact download/upload) and DNS (`kube-dns`).
  - No platform service (except the public-facing `submission-api` and `leaderboard-ws`) is exposed to the internet.

## RBAC Breakdown

Strict Role-Based Access Control limits what the orchestration services can do:

- **`sandbox-orchestrator`**: Bound to a `Role` in the `sandboxes` namespace. Permitted to `create`, `get`, `list`, `watch`, and `delete` `pods` and `get` `pods/log`.
- **`build-service`**: Bound to a `Role` in the `builds` namespace. Permitted to `create`, `get`, `list`, `watch`, and `delete` `pods` and `get` `pods/log`.
- **`bot-orchestrator`**: Bound to two `Roles`:
  - In `bots`: Permitted to `create`, `watch`, and `delete` `jobs`.
  - In `sandboxes`: Permitted to `delete` `pods` (for cleanup post-test).

Services like `submission-api` and `telemetry-ingester` do not have specialized K8s RBAC roles, as they do not interact with the Kubernetes API server.

## KEDA Scaling

KEDA (Kubernetes Event-driven Autoscaling) is used to scale platform services based on Redpanda queue depth.

`ScaledObject` resources are defined in the Helm chart. For example, `build-service`, `sandbox-orchestrator`, and `bot-orchestrator` can scale out horizontally if their respective Kafka consumer group lag crosses a defined threshold, ensuring high throughput during mass submission events.
