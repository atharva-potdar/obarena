# Deployment Guide

The OBARENA platform is designed to be easily deployable in both local development environments and production cloud environments using a unified set of tools (`just`, Helm, Ansible, Terraform).

## The `k0s kubectl` Convention

**CRITICAL RULE**: Whether in development or production (if using k0s as the distribution), all cluster interactions must use the following pattern:

```bash
KUBECONFIG=~/.kube/config k0s kubectl <command>
```

**Why?** k0s ships with its own isolated `kubectl` binary to avoid conflicting with system-wide installations. We explicitly pass the `KUBECONFIG` to ensure it targets the correct cluster context, especially when switching between local dev and remote staging/prod clusters. Bare `kubectl` commands will likely fail or target the wrong cluster.

---

## Local Development Workflow

The local workflow uses Ansible to provision a single-node k0s cluster and `just` as a command runner for building, loading, and deploying.

### 1. Initial Infrastructure Bootstrap

Run the Ansible playbook to install k0s, Cilium, Helm, and Longhorn:

```bash
ansible-playbook -i inventory.ini site.yml
```

### 2. Full Deployment Pipeline

The `just` command handles the entire dev pipeline:

```bash
just
```

`just` (or equivalently `just dev-up`) runs the following sequence:
1. `just build`: Compiles Go binaries, builds Docker images, and loads them into the k0s containerd runtime.
2. `just prefetch`: Pulls third-party dependency containers (Redpanda, TimescaleDB, Redis, etc.) into the k0s container cache.
3. `just infra-up`: Installs KEDA and deploys the core platform Helm chart with dev configurations.
4. `just smoke-test`: Validates namespace readiness, network policies, RBAC, and pod health.

### 3. Port Forwarding

To access services locally, port-forward using the `k0s kubectl` pattern:

```bash
# Submission API (for POST /submissions)
KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/submission-api 8080:8080

# SeaweedFS (for S3 presigned URL uploads)
KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/seaweedfs 8333:8333

# Leaderboard UI (for viewing results)
KUBECONFIG=~/.kube/config k0s kubectl port-forward -n platform svc/leaderboard-ws 8090:8090
```

### 4. Teardown

To completely wipe the platform resources (keeping k0s running):

```bash
just dev-teardown
```

---

## Production Workflow

The production workflow targets AWS (EKS) and is managed via Terraform for infrastructure and Helm for deployment.

### 1. Infrastructure Provisioning

```bash
just tf-init
just tf-up
```

This provisions an EKS cluster, managed node groups (split by platform, sandboxes, and bots taints), a VPC, and an ECR registry.

### 2. Image Build & Push

```bash
just push
```

Builds the images for the `linux/amd64` or `linux/arm64` architecture (depending on target) and pushes them to the provisioned ECR registry.

### 3. Application Deployment

```bash
just helm-deploy
```

Deploys the Helm chart, passing the production values file (`values-prod.yaml`), which configures node selectors, tolerations, HPA, and production resource limits.

---

## Troubleshooting

### 1. Checking Pod Status

If services are failing or the smoke test hangs, check pod health:

```bash
KUBECONFIG=~/.kube/config k0s kubectl get pods -A
```

Look for pods in `CrashLoopBackOff` or `Pending` states. 
- **Pending sandboxes/builds**: Usually indicates a lack of resources or unschedulable nodes.
- **Evicted pods**: Node disk pressure or memory pressure.

### 2. Inspecting Logs

To view logs for a failing platform service:

```bash
KUBECONFIG=~/.kube/config k0s kubectl logs -n platform deployment/<service-name> --tail=100 -f
```

To view logs for a bot-runner Job (which runs in the `bots` namespace):

```bash
KUBECONFIG=~/.kube/config k0s kubectl logs -n bots -l app.kubernetes.io/name=bot-runner -f
```

### 3. Deep Dive into Sandbox Failures

Sandboxes are ephemeral, but the orchestrator leaves them intact if the wait for readiness fails (until the parent context cancels). To inspect a failed sandbox:

```bash
KUBECONFIG=~/.kube/config k0s kubectl describe pod <pod-name> -n sandboxes
KUBECONFIG=~/.kube/config k0s kubectl logs <pod-name> -n sandboxes -c init-download
KUBECONFIG=~/.kube/config k0s kubectl logs <pod-name> -n sandboxes -c sandbox
```

### 4. Network Policy Drops

If services cannot communicate, it may be a Cilium NetworkPolicy issue. Check Hubble (if installed) or use the cilium agent to check for drops:

```bash
KUBECONFIG=~/.kube/config k0s kubectl -n kube-system exec ds/cilium -- cilium monitor --type drop
```
