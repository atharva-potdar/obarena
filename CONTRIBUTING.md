# Contributing to OBARENA

We welcome contributions! This document outlines the standards, conventions, and processes for contributing to the OBARENA platform.

## Code Style & Conventions

### Go Services

- **Language Version**: Go 1.26.
- **Formatting**: Code must be formatted using `gofmt`.
- **Linting**: Ensure code passes `go vet ./...`. No compiler warnings are allowed.
- **Logging**: Use the standard library structured logging package (`log/slog`) across all services. Do not use third-party logging frameworks unless absolutely necessary for a specific dependency.
- **Kafka Client**: We exclusively use the `franz-go` library (`github.com/twmb/franz-go/pkg/kgo`) for interacting with Redpanda.
- **Documentation**: Use short, single-line comments for non-obvious logic. Write clear package-level and function-level docstrings for exported APIs. Avoid writing massive, multi-paragraph comment walls. The code should be readable and its intent obvious.

### Infrastructure

- **Kubernetes**: All interactions must use `KUBECONFIG=~/.kube/config k0s kubectl` to avoid environment bleed.
- **Helm**: Ensure templates pass `helm lint infra/helm/obarena-platform/`. Use `_helpers.tpl` for shared logic.
- **Terraform**: Run `terraform fmt` before committing.

## Commit Format

We enforce semantic commit messages to automatically generate changelogs and version bumps.

Format:
```
<type>(<scope>): <subject>

- <bulleted detail 1>
- <bulleted detail 2>
```

**Types**:
- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Changes that do not affect the meaning of the code (white-space, formatting)
- `refactor`: A code change that neither fixes a bug nor adds a feature
- `test`: Adding missing tests or correcting existing tests
- `chore`: Changes to the build process or auxiliary tools and libraries

**Scopes**:
Use the service or component name (e.g., `build-service`, `bot-runner`, `helm`, `terraform`).

**Example**:
```
fix(bot-runner): correct websocket closure handling

- Prevent panic when remote unexpectedly disconnects
- Add robust context cancellation for phase 2 load test
```

## Build and Test Workflow

Before pushing code, ensure you run:

1. **Verify Compilation**:
   ```bash
   go build -o /dev/null ./...
   ```
2. **Lint**:
   ```bash
   go vet ./...
   ```
3. **Local Dev Deployment**:
   ```bash
   just
   ```
   This will build images, load them into the local k0s cluster, deploy via Helm, and run a smoke test. The smoke test must pass before a PR is opened.

## Pull Request Process

1. Fork the repository and create a new branch from `main`.
2. Make your changes following the code style guidelines.
3. Ensure all local tests (`just`) pass.
4. Push your branch and open a Pull Request against `main`.
5. Ensure the PR title follows the semantic commit format (it will become the squash merge commit).
6. Request review from a maintainer.

## Directory Structure Overview

```text
.
├── services/               # Go microservices
│   ├── submission-api/     # HTTP ingress
│   ├── build-service/      # Compilation
│   ├── sandbox-orchestrator/ # Pod execution
│   ├── bot-orchestrator/   # Test execution management
│   ├── bot-runner/         # Load testing binary (runs in Jobs)
│   ├── telemetry-ingester/ # Scoring and TSDB writes
│   ├── leaderboard-ws/     # WebSocket UI server
│   └── stub/               # Reference orderbook engine
├── infra/
│   ├── helm/               # Source of truth for k8s deployment
│   └── terraform/          # AWS EKS provisioning
├── scripts/                # Utility scripts
├── docs/                   # System documentation
├── ANSIBLE.md              # Ansible node bootstrapping guide
├── CLAUDE.md               # AI assistant context
├── justfile                # Command runner definitions
└── site.yml                # Ansible k0s playbook
```
