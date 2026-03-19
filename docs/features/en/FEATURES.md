# 🚀 Telekube — Feature Overview

> A Telegram Bot for managing Kubernetes clusters, ArgoCD, Helm releases, and real-time monitoring — all from your chat.

---

## 📋 Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Kubernetes Module](#kubernetes-module)
3. [ArgoCD Module](#argocd-module)
4. [Helm Module](#helm-module)
5. [Watcher Module (Real-Time Monitoring)](#watcher-module)
6. [Briefing Module (Daily Reports)](#briefing-module)
7. [Approval Module (Approval Workflows)](#approval-module)
8. [Incident Module (Incident Timeline)](#incident-module)
9. [AlertManager Module](#alertmanager-module)
10. [RBAC Management Module](#rbac-management-module)
11. [Notification Preferences Module](#notification-preferences-module)
12. [RBAC System](#rbac-system)
13. [Audit System](#audit-system)
14. [Leader Election](#leader-election)
15. [Multi-Cluster Support](#multi-cluster-support)
16. [Security](#security)
17. [Observability](#observability)

---

## Architecture Overview

Telekube is built on a **modular plugin architecture** — each feature is a self-contained module:

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
│  ┌────────┐ ┌────────┐ ┌────────────┐           │
│  │Approval│ │Incident│ │AlertManager│           │
│  └────────┘ └────────┘ └────────────┘           │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌────┐ ┌───────┐ ┌────────────────┐  │
│  │ RBAC │ │Audit│ │Cluster│ │Leader Election │  │
│  │Engine│ │Logger││Manager│ │   (K8s Lease)  │  │
│  └──────┘ └────┘ └───────┘ └────────────────┘  │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────────┐ ┌───────┐               │
│  │SQLite│ │PostgreSQL │ │ Redis │               │
│  └──────┘ └──────────┘ └───────┘               │
└─────────────────────────────────────────────────┘
```

---

## Kubernetes Module

The core module for managing Kubernetes clusters directly from Telegram.

### Available Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `/pods` | List pods by namespace | `kubernetes.pods.list` |
| `/pods <namespace>` | View pods in a specific namespace | `kubernetes.pods.list` |
| `/logs <pod>` | View pod logs | `kubernetes.pods.logs` |
| `/events <pod>` | View pod events | `kubernetes.pods.events` |
| `/top` | Show pod CPU/RAM usage | `kubernetes.metrics.view` |
| `/top nodes` | Show node CPU/RAM usage | `kubernetes.metrics.view` |
| `/scale` | Scale deployment/statefulset replicas | `kubernetes.deployments.scale` |
| `/restart <pod>` | Restart a pod (delete + recreate) | `kubernetes.pods.restart` |
| `/nodes` | List and manage cluster nodes | `kubernetes.nodes.view` |
| `/quota` | Show namespace resource quotas | `kubernetes.quota.view` |
| `/namespaces` | List all namespaces | `kubernetes.namespaces.list` |
| `/deploys [ns]` | List deployments in namespace | `kubernetes.deployments.list` |
| `/cronjobs [ns]` | List CronJob status | `kubernetes.cronjobs.list` |

### Detailed Features

#### 📦 Pod Management
- **List pods** with status emojis (✅ Running, 🟡 Pending, 🔴 Failed, ⚪ Unknown)
- **Pagination** when many pods exist — navigate with ◀️ ▶️ buttons
- **Pod detail view**: IP, node, uptime, restart count, container statuses
- **Log viewer**: Select container, scroll through logs (📄 More / ⬆️ Previous)
- **Event viewer**: Kubernetes events related to the pod
- **Pod restart**: Delete pod for controller recreation (with confirmation ✅/❌)

#### 📊 Resource Metrics
- **Top pods**: CPU/RAM usage sorted by consumption
- **Top nodes**: Aggregate CPU/RAM, allocatable vs used
- **Visual progress bars** for usage percentages

#### ⚖️ Scale Operations
- **Select namespace → deployment/statefulset → new replica count**
- **Confirmation before scaling** (prevents accidental changes)
- **Scale to 0** support (take workloads offline)

#### 🖥️ Node Management
- **Node details**: CPU, memory, OS, kubelet version, conditions
- **Cordon/Uncordon**: Mark node as unschedulable or schedulable
- **Drain**: Evict pods and move them to other nodes (with grace period)
- **Top pods on node**: View pods running on a specific node

---

## ArgoCD Module

Full ArgoCD GitOps integration — manage applications, sync, rollback, and deployment freezes.

### Available Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `/apps` | List ArgoCD applications | `argocd.apps.list` |
| `/dashboard` | GitOps status dashboard | `argocd.apps.list` |
| `/freeze` | Manage deployment freeze | `argocd.freeze.manage` |

### Detailed Features

#### 📱 Application Management
- **List apps** with sync/health status (✅ Synced, 🟡 OutOfSync, 🔴 Degraded)
- **App detail view**: Git revision, source repo, target revision, resources
- **Multi-instance support** — switch between ArgoCD instances easily

#### 🔄 Sync Operations
- **Sync Now**: Synchronize app with Git
- **Sync with Prune**: Remove resources no longer in Git
- **Force Sync**: Override pending sync operations
- **Confirmation required** before execution

#### ⏪ Rollback
- **Select previous revision** to rollback to
- **Revision history** with timestamps and status

#### 🧊 Deployment Freeze
- **Create freeze window**: Block sync/rollback during a time period
- **Scope selection**: All apps or specific apps
- **Duration options**: 30min, 1h, 2h, 4h, 8h, 24h
- **Thaw**: End freeze early
- **Freeze history**: View previous freezes

#### 📊 GitOps Dashboard
- Aggregate status of all apps across all instances
- **Quick filters**: View OutOfSync or Degraded apps
- **Refresh** to update status

---

## Helm Module

Manage Helm releases — list, view details, and rollback.

### Available Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `/helm` | List and manage Helm releases | `helm.releases.list` |

### Detailed Features

- **Select namespace** → display releases with status emojis
- **Release detail**: Chart version, app version, revision, last deployed
- **Revision history**: View all revisions with timestamps
- **Rollback**: Select a previous revision (requires `helm.releases.rollback` permission)
- **Refresh**: Update release list

---

## Watcher Module

Real-time Kubernetes monitoring — sends Telegram alerts when issues are detected.

### Watchers

| Watcher | Monitors | Alerts On |
|---------|----------|-----------|
| **PodWatcher** | Pods across all clusters | OOMKilled, CrashLoopBackOff, ImagePullBackOff, PendingTooLong, Evicted, HighRestarts |
| **NodeWatcher** | Nodes across all clusters | NotReady, Unreachable, DiskPressure, MemoryPressure, PIDPressure |
| **CronJobWatcher** | CronJobs | Job failed, deadline exceeded |
| **CertWatcher** | Certificates (cert-manager) | Certificate nearing expiry, renewal failed |
| **PVCWatcher** | PersistentVolumeClaims | High disk usage (Warning ≥80%, Critical ≥90%) |
| **ArgoCDWatcher** | ArgoCD applications | App OutOfSync or Degraded |

### Detailed Features

#### 🚨 Alert System
- **Alert deduplication**: No duplicate alerts within cooldown period (5 minutes default)
- **Alert severity levels**: 🔴 Critical, ⚠️ Warning, ℹ️ Info
- **Mute button**: Press 🔇 to suppress alerts for 1 hour
- **Inline action buttons**: Jump from alert → view top pods, details

#### 📊 PVC Usage Monitoring
- **Visual progress bar**: `[████████░░] 85%`
- **Capacity display**: `8.5GiB / 10.0GiB`
- **Annotation support**: Reads `telekube.io/pvc-usage-bytes`
- **Namespace exclusion**: Configurable `exclude_namespaces`

#### ⚡ Leader-Only Execution
- Watchers run only on the elected leader replica (prevents duplicate alerts)
- Automatically stops when leadership is lost

#### 📜 Custom Alert Rules
- **Config-driven rules**: Define custom alerting rules in YAML config
- **Supported conditions**:
  - `pod_restart_count` — Alert when container restarts exceed threshold
  - `pod_pending_duration` — Alert when pods stay Pending beyond time window
  - `namespace_quota_percentage` — Alert when resource quota exceeds percentage
  - `deployment_unavailable` — Alert when deployments have zero available replicas
- **Scoped evaluation**: Limit rules to specific clusters and/or namespaces
- **10-minute deduplication**: Prevents repeated alerts for the same issue
- **Configurable notifications**: Route alerts to specific chats
- **Full audit logging**: All triggered custom alerts are audit-logged

---

## Briefing Module

Daily cluster health reports — sent automatically at a configured time.

### Configuration

```yaml
modules:
  briefing:
    enabled: true
    schedule: "0 8 * * *"     # Every day at 8:00 AM
    timezone: "Asia/Ho_Chi_Minh"
```

### Report Contents

- **Nodes status**: ✅ 3/3 Ready or 🔴 2/5 Ready
- **Pods summary**: Running, Failed, Pending counts
- **Resource usage**: Average CPU and RAM across the cluster
- **Activity summary**: Total actions, restarts, scales, deploys in the last 24h

### Features
- **Timezone support** for cron scheduling
- **Multi-chat delivery**: Send to multiple groups/users simultaneously
- **Leader-only execution** (prevents duplicate reports)

---

## Approval Module

Approval workflows for sensitive operations (production sync, rollback, scale).

### Detailed Features

- **Rule-based triggers**: Configure which actions and clusters require approval
- **Multi-approval**: Require N approvers (e.g., 2 out of 3 admins)
- **Approver role filtering**: Only specified roles can approve
- **Self-approval prevention**: Requester cannot approve their own request
- **Auto-expiry**: Requests expire automatically (default 30 minutes)
- **Cancellation**: Requester can cancel pending requests
- **Background expiry worker**: Automatically marks expired requests

### Workflow

```
Operator → /sync prod-app
         → "Requires 2 admin approvals"
         → Admin A presses ✅ Approve
         → Admin B presses ✅ Approve
         → Sync executes automatically
```

---

## Incident Module

Build incident timelines — aggregate K8s events and audit logs chronologically.

### Commands

| Command | Description |
|---------|-------------|
| `/incident` | Open the incident timeline builder |

### Features

- **Select namespace** and **time window** (15min, 30min, 1h, 4h)
- **Combines two data sources**:
  - Kubernetes Events (⚠️ Warning, 📦 Scheduled, 💥 OOMKilling, 🔁 BackOff)
  - Audit Log (👤 User actions: scale, restart, deploy)
- **Chronological ordering** — view incident progression in sequence
- **Capped at 30 events** for display (prevents overly long messages)

---

## AlertManager Module

Receive webhooks from Prometheus AlertManager and forward alerts to Telegram.

### Features

- **Webhook receiver**: HTTP endpoint for Prometheus alert payloads
- **Bearer token authentication**: Secure webhook requests
- **Alert formatting**: Display severity, namespace, pod, summary, description
- **Silence from Telegram**: Press 🔇 Silence 1h / 4h → creates silence via AlertManager API
- **Acknowledge**: Press ✅ Ack to acknowledge
- **Critical-first sorting**: Alerts sorted by severity (critical → warning → info)
- **Resolved notifications**: Notifies when alerts are resolved

---

## RBAC Management Module

Admin-only Telegram command (`/rbac`) for managing user roles interactively.

### Features

- **List users**: View all registered users with their assigned roles
- **List roles**: View all built-in (5) and custom roles with permissions
- **Assign role**: Select a user, then assign a new role via inline buttons
- **Revoke role/binding**: Revoke dynamic role bindings or reset to viewer
- **User details**: View a user’s flat role, dynamic bindings, and super-admin status
- **Permission gated**: All actions require `admin.rbac.manage` permission
- **Full audit logging**: Every role change is recorded in the audit log

---

## Notification Preferences Module

Per-user notification preferences managed via the `/notify` Telegram command.

### Features

- **Severity filter**: Choose minimum severity level (`info`, `warning`, `critical`)
- **Quiet hours**: Suppress non-critical alerts during configured time windows
  - Options: 22:00–08:00, 23:00–07:00, 00:00–06:00, or disabled
- **Cluster muting**: Toggle alerts on/off for specific clusters
- **Per-user settings**: Each user configures their own preferences independently
- **Persistent storage**: Preferences saved in SQLite/PostgreSQL
- **Watcher integration**: `ShouldNotify()` function for watchers to check before sending

---

## RBAC System

Role-Based Access Control — fine-grained permission management.

### Default Roles

| Role | Permissions |
|------|-------------|
| **viewer** | View pods, logs, events, metrics, nodes (read-only) |
| **operator** | Viewer + restart pods, scale deployments, view ArgoCD diff, Helm list |
| **admin** | Operator + manage nodes (cordon/drain), rollback Helm, manage freeze, sync/rollback ArgoCD, RBAC management |
| **on-call** | Operator + rollback + drain (auto-expires) |
| **super-admin** | Full access — unrestricted |

### Advanced Features

- **Super Admin bypass**: Admin IDs in config always have full permissions
- **Phase 4 Policy Rules**: Fine-grained RBAC with allow/deny rules
  - Match by module, resource, action, cluster, namespace
  - **Deny overrides allow**
  - Wildcard `*` support
- **Custom Roles**: Create custom role definitions
- **Role Bindings**: Assign roles to users with optional expiry
- **Fallback**: If no dynamic bindings match → uses legacy flat role
- **Telegram management**: `/rbac` command for admin role management

---

## Audit System

Comprehensive logging of all user operations.

### Features

- **Buffered async writes**: Non-blocking log writes via channel (1000-entry buffer)
- **Batch flush**: Write to database in batches for optimal performance
- **Structured entries**: User, action, resource, cluster, namespace, status, details
- **Query API**: Search audit logs with filters (page, page_size, user, action, cluster)
- **Middleware auto-logging**: Automatic audit logging for every command
- **Concurrent safety**: Thread-safe for multiple goroutines writing simultaneously

---

## Leader Election

Uses Kubernetes Leases to elect a leader — ensures only one replica runs watchers and briefing.

### How It Works

```
Pod A: Leader ← Runs watchers, briefing scheduler
Pod B: Follower → Standby, ready for takeover
Pod C: Follower → Standby
```

- **Lease-based**: Uses Kubernetes Lease objects
- **Auto-recovery**: When leader is lost, a follower automatically takes over
- **Callbacks**: `OnStartedLeading` → start watchers; `OnStoppedLeading` → stop watchers

---

## Multi-Cluster Support

Manage multiple Kubernetes clusters simultaneously.

### Features

- **Cluster selector**: Select cluster via inline buttons on `/start`
- **Per-user context**: Each user operates independently (user A on prod, user B on staging)
- **Display names**: Display-friendly names separate from technical names
- **Health checks**: Periodic cluster connectivity checks
- **Lazy initialization**: Kubernetes clients created on demand

---

## Security

### Authentication & Authorization
- **Telegram user ID**: Authentication based on Telegram identity
- **Per-command RBAC**: Each command requires a specific permission
- **Admin IDs**: Super admin list in configuration
- **Allowed chats**: Restrict bot to approved chats only

### Rate Limiting
- **Per-user limits**: Configurable commands per user (default 60/minute)
- **Sliding window algorithm**: Fair and accurate rate limiting

### Middleware Chain
```
Request → Recovery → Rate Limit → Auth → Audit → Handler
```

---

## Observability

### Health & Readiness
- `/healthz` — Check if the bot is running
- `/readyz` — Check dependency health (DB, clusters)

### Structured Logging
- **Zap JSON logger** with trace correlation
- **Log levels**: Debug, Info, Warn, Error
- **Sensitive data redaction**: No tokens or passwords in logs

### Module Health Status
- Each module reports its health: `healthy` / `unhealthy` / `unknown`
- Aggregated health from all modules

---

## Storage Backends

| Backend | Used For | Best For |
|---------|----------|----------|
| **SQLite** | Users, RBAC, Audit, Approvals, Freezes, Notification Prefs | Development, single instance |
| **PostgreSQL** | Same as SQLite but scalable | Production, multi-instance |
| **Redis** | Cache, rate limiting, idempotency | All environments |

---

## Internationalization (i18n)

- **Multi-language support** via `pkg/i18n` package
- **Vietnamese messages** for briefing reports

---

*Telekube — Kubernetes management, right in your chat. 🚀*
