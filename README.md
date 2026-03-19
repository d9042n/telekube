<p align="center">
  <img src="docs/assets/logo.png" width="180" alt="Telekube Logo">
</p>

<h1 align="center">Telekube</h1>
<p align="center">
  <strong>Kubernetes & ArgoCD Command Center for Telegram</strong>
</p>

<p align="center">
  <a href="https://github.com/d9042n/telekube/releases"><img src="https://img.shields.io/github/v/release/d9042n/telekube?style=flat-square" alt="Release"></a>
  <a href="https://github.com/d9042n/telekube/actions/workflows/ci.yaml"><img src="https://img.shields.io/github/actions/workflow/status/d9042n/telekube/ci.yaml?branch=main&style=flat-square&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/d9042n/telekube?style=flat-square" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/d9042n/telekube"><img src="https://goreportcard.com/badge/github.com/d9042n/telekube?style=flat-square" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/d9042n/telekube"><img src="https://img.shields.io/badge/go-reference-blue?style=flat-square" alt="Go Reference"></a>
</p>

---

**Telekube** turns Telegram into a powerful Command Center for Kubernetes and ArgoCD.
Manage clusters, deploy applications, monitor infrastructure, and respond to incidents — all from your phone.

## ✨ Features

- **Multi-Cluster Kubernetes** — Pods, logs, events, metrics, scaling, node management
- **ArgoCD GitOps** — Sync, rollback, diff preview, deployment status dashboard
- **Proactive Monitoring** — Real-time alerts for OOMKills, CrashLoopBackOff, node failures, CronJob failures, cert expiry, PVC usage
- **Custom Alert Rules** — Config-driven alerting (restart count, pending duration, quota, unavailable deployments)
- **Helm Management** — List, inspect, rollback Helm releases
- **Incident Response** — Auto-generated incident timelines combining K8s events and audit logs
- **Approval Workflows** — Multi-step approval for sensitive operations in production
- **Deployment Freeze** — Block deployments during maintenance windows
- **Daily Briefings** — Scheduled cluster health summaries via cron
- **AlertManager Integration** — Receive Prometheus alerts and silence them from Telegram
- **Enterprise Governance** — RBAC with fine-grained policy rules, full audit logging, `/rbac` admin command
- **Notification Preferences** — Per-user severity filter, quiet hours, cluster muting via `/notify`
- **Leader Election** — HA mode via Kubernetes Leases for multi-replica deployments
- **i18n** — English and Vietnamese UI support

---

## 🚀 Quick Start

### Prerequisites

- Go 1.25+ or Docker
- Telegram Bot token from [@BotFather](https://t.me/BotFather)
- Access to a Kubernetes cluster

### Option 1: Interactive Setup (Recommended)

```bash
git clone https://github.com/d9042n/telekube.git
cd telekube

make build
./bin/telekube setup              # Interactive setup wizard
./bin/telekube serve --config configs/config.yaml
```

### Option 2: Manual Configuration

```bash
git clone https://github.com/d9042n/telekube.git
cd telekube

cp configs/config.example.yaml configs/config.yaml
# Edit configs/config.yaml with your settings

# Or use environment variables:
export TELEKUBE_TELEGRAM_TOKEN="bot123456:ABC..."
export TELEKUBE_TELEGRAM_ADMIN_IDS="123456789"

make build && make run
```

### Option 3: Helm (Kubernetes)

```bash
helm install telekube deploy/helm/telekube \
  --namespace telekube \
  --create-namespace \
  --set config.telegram.token=$TELEKUBE_TELEGRAM_TOKEN \
  --set config.telegram.adminIDs="{123456789}"
```

### Option 4: Docker

```bash
docker run --rm -it \
  -e TELEKUBE_TELEGRAM_TOKEN="your-token" \
  -e TELEKUBE_TELEGRAM_ADMIN_IDS="123456789" \
  -v ~/.kube:/root/.kube:ro \
  ghcr.io/d9042n/telekube:latest
```

---

## 📋 Commands

| Command | Description | Min Role |
|---------|-------------|----------|
| `/start` | Welcome & cluster selection | all |
| `/help` | Show available commands | all |
| `/clusters` | Switch active cluster | all |
| `/pods [ns]` | List pods in namespace | viewer |
| `/logs <pod>` | View pod logs | viewer |
| `/events <pod>` | View pod events | viewer |
| `/top` | Pod/node CPU & memory usage | viewer |
| `/nodes` | List and manage cluster nodes | viewer |
| `/quota` | Show namespace resource quotas | viewer |
| `/namespaces` | List all namespaces | viewer |
| `/deploys [ns]` | List deployments | viewer |
| `/cronjobs [ns]` | CronJob status | viewer |
| `/restart <pod>` | Restart a pod | operator |
| `/scale <deploy>` | Scale deployment replicas | operator |
| `/helm` | List Helm releases | operator |
| `/apps` | ArgoCD applications | viewer |
| `/dashboard` | ArgoCD GitOps status dashboard | viewer |
| `/incident` | Build incident timeline | viewer |
| `/freeze` | Set deployment freeze window | admin |
| `/audit` | View audit log | admin |
| `/rbac` | Manage user roles and bindings | admin |
| `/notify` | Manage your notification preferences | all |

> **Note:** Some operations (sync, rollback, cordon, drain, approve) are triggered via **inline keyboard buttons** from within command results.

---

## 🏗️ Architecture

Telekube is built on a **modular plugin architecture** — each feature is a self-contained module that can be enabled or disabled independently.

```
┌─────────────────────────────────────────────────┐
│                  Telegram Bot                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
│  │Middleware │ │ Handlers │ │  Keyboard Builder│ │
│  │(Auth,Rate,│ │(/start,  │ │  (Inline Buttons)│ │
│  │ Audit)   │ │ /help)   │ │                  │ │
│  └──────────┘ └──────────┘ └──────────────────┘ │
├─────────────────────────────────────────────────┤
│              Module Registry                     │
│  ┌──────┐ ┌──────┐ ┌────┐ ┌───────┐ ┌────────┐ │
│  │ K8s  │ │ArgoCD│ │Helm│ │Watcher│ │Briefing│ │
│  └──────┘ └──────┘ └────┘ └───────┘ └────────┘ │
│  ┌────────┐ ┌────────┐ ┌────────────┐ ┌──────┐ │
│  │Approval│ │Incident│ │AlertManager│ │Notify│ │
│  └────────┘ └────────┘ └────────────┘ └──────┘ │
│  ┌──────────┐                                   │
│  │ RBAC Mgmt│                                   │
│  └──────────┘                                   │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌─────┐ ┌───────┐ ┌────────────────┐ │
│  │ RBAC │ │Audit│ │Cluster│ │Leader Election │ │
│  │Engine│ │ Log │ │Manager│ │  (K8s Lease)   │ │
│  └──────┘ └─────┘ └───────┘ └────────────────┘ │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────────┐ ┌───────┐               │
│  │SQLite│ │PostgreSQL│ │ Redis │               │
│  └──────┘ └──────────┘ └───────┘               │
└─────────────────────────────────────────────────┘
```

### Project Structure

```
cmd/telekube/          # Entrypoint (serve + setup wizard)
internal/
  app/                 # Application lifecycle, DI wiring
  bot/                 # Telegram bot, middleware, handlers
  module/              # Module system — pluggable features
    kubernetes/        # K8s operations (pods, nodes, deployments)
    argocd/            # ArgoCD GitOps integration
    watcher/           # Real-time monitoring watchers + custom rules
    helm/              # Helm release management
    incident/          # Incident response timeline
    approval/          # Multi-step approval workflows
    briefing/          # Scheduled daily briefings
    alertmanager/      # Prometheus AlertManager webhook
    rbacmod/           # RBAC management (/rbac command)
    notify/            # Notification preferences (/notify command)
  cluster/             # Multi-cluster manager
  rbac/                # Role-based access control engine
  audit/               # Audit logging
  storage/             # Storage abstraction (SQLite + PostgreSQL)
  entity/              # Domain models
  leader/              # Leader election (HA mode)
pkg/
  i18n/                # Internationalization (en, vi)
  kube/                # Kubernetes client factory
  telegram/            # Telegram formatting utilities
  logger/              # Structured logging (Zap)
  version/             # Build version info
  health/              # Health check server
  redis/               # Redis client
  argocd/              # ArgoCD API client
```

---

## ⚙️ Configuration

Configuration is loaded in the following order of precedence:

1. Environment variables (`TELEKUBE_*`)
2. Config file (`configs/config.yaml`)
3. Defaults

See [`configs/config.example.yaml`](configs/config.example.yaml) for all available options.

### Key Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TELEKUBE_TELEGRAM_TOKEN` | Telegram bot token | Yes |
| `TELEKUBE_TELEGRAM_ADMIN_IDS` | Admin user IDs (comma-separated) | Yes |
| `TELEKUBE_STORAGE_BACKEND` | `sqlite` or `postgres` | No (default: `sqlite`) |
| `TELEKUBE_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | No (default: `info`) |

### Module Toggles

Each module can be independently enabled or disabled in the config:

```yaml
modules:
  kubernetes:    { enabled: true }
  argocd:        { enabled: false }
  watcher:       { enabled: false }
  helm:          { enabled: false }
  approval:      { enabled: false }
  incident:      { enabled: false }
  alertmanager:  { enabled: false }
  notify:        { enabled: false }
  briefing:
    enabled: false
    schedule: "0 8 * * *"        # 8:00 AM daily (cron)
    timezone: "Asia/Ho_Chi_Minh"
```

---

## 🔐 RBAC

Telekube ships with **five** built-in roles:

| Role | Permissions |
|------|-------------|
| **viewer** | Read-only — list pods, logs, events, metrics, nodes |
| **operator** | Viewer + write ops — restart, scale, sync diff, Helm list |
| **admin** | Operator + admin ops — audit, RBAC, freeze, node cordon/drain, sync, rollback |
| **on-call** | Operator + rollback + drain (auto-expires) |
| **super-admin** | Full access — unrestricted |

Telegram bot admins (configured via `admin_ids`) always have the **super-admin** role.

Advanced fine-grained policy rules are supported with allow/deny matching by module, resource, action, cluster, and namespace.

Use `/rbac` in Telegram to manage user roles.

---

## 🛠️ Development

```bash
make build             # Build the binary
make run               # Build and run with config
make test              # Run all tests with race detection
make test-unit         # Unit tests only (no Docker required)
make test-e2e          # E2E tests (requires Docker)
make test-coverage     # Generate HTML coverage report
make lint              # Run golangci-lint
make fmt               # Format code (gofmt + goimports)
make vuln              # Run govulncheck
make docker-build      # Build Docker image
make helm-lint         # Lint the Helm chart
make help              # Show all available targets
```

---

## 📖 Documentation

| Document | Description |
|----------|-------------|
| [Features (EN)](docs/features/en/FEATURES.md) | Complete feature overview |
| [Guides (EN)](docs/guides/en/) | Step-by-step guides (installation, configuration, modules) |
| [Features (VI)](docs/features/vi/) | Tổng quan tính năng (Tiếng Việt) |
| [Guides (VI)](docs/guides/vi/) | Hướng dẫn chi tiết (Tiếng Việt) |
| [Example Config](configs/config.example.yaml) | Annotated configuration reference |

---

## 🤝 Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for your changes
4. Ensure all checks pass (`make lint && make test`)
5. Commit your changes (`git commit -m 'feat: add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

---

## 📄 License

Apache License 2.0 — see [LICENSE](LICENSE).

---

<p align="center">
  Built with ❤️ using Go, Telebot, and Kubernetes client-go
</p>
