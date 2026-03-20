# Telekube — Commands & Features Reference

> Complete reference of all commands, background features, and their capabilities.

---

## Command Summary

### 🤖 Core Commands (always available)

| Command | Description | Permission |
|---------|-------------|------------|
| `/start` | Welcome message + cluster selection | None |
| `/help` | Dynamic help (grouped by module, filtered by RBAC) | None |
| `/clusters` | Switch active cluster | None |
| `/audit` | View audit log (paginated, 20 per page) | None |

### ☸️ Kubernetes (12 commands)

| Command | Description | Permission |
|---------|-------------|------------|
| `/pods` | List pods in a namespace (interactive namespace selection) | `kubernetes.pods.list` |
| `/logs <pod>` | View pod logs (50/100/200/500 lines, previous container) | `kubernetes.pods.logs` |
| `/events <pod>` | View Kubernetes events for a pod | `kubernetes.pods.events` |
| `/restart <pod>` | Delete/restart a pod (with optional approval gate) | `kubernetes.pods.restart` |
| `/top` | Pod resource usage (CPU/memory via Metrics Server) | `kubernetes.metrics.view` |
| `/top nodes` | Node resource usage (CPU/memory) | `kubernetes.metrics.view` |
| `/scale` | Scale deployment/statefulset replicas (interactive flow) | `kubernetes.deployments.scale` |
| `/nodes` | List nodes with status, CPU, memory, disk. Cordon/uncordon/drain | `kubernetes.nodes.view` |
| `/quota` | Show namespace resource quotas and LimitRanges | `kubernetes.quota.view` |
| `/namespaces` | List all namespaces | `kubernetes.namespaces.list` |
| `/deploys` | List deployments with replica status | `kubernetes.deployments.list` |
| `/cronjobs` | List CronJobs with last schedule and status | `kubernetes.cronjobs.list` |

### ⎈ Helm (1 command)

| Command | Description | Permission |
|---------|-------------|------------|
| `/helm` | List releases → detail → rollback (interactive) | `helm.releases.list` |

**Interactive flow:**
1. Select namespace (dynamically fetched from cluster)
2. View release list with chart versions
3. Click release → view detail (status, revision, values)
4. Rollback to previous revision (`helm.releases.rollback` permission)

### 🚨 Incident (1 command)

| Command | Description | Permission |
|---------|-------------|------------|
| `/incident` | Build incident timeline from K8s events + audit log | `kubernetes.pods.events` |

**Interactive flow:**
1. Select namespace (dynamically fetched, max 8, excludes system namespaces)
2. Select time window (15min / 30min / 1h / 4h)
3. View merged chronological timeline with severity indicators

### 🔄 ArgoCD (3 commands) — requires ArgoCD config

| Command | Description | Permission |
|---------|-------------|------------|
| `/apps` | List ArgoCD applications with sync/health status | `argocd.apps.list` |
| `/dashboard` | GitOps status dashboard across all instances | `argocd.apps.list` |
| `/freeze` | Manage deployment freeze (block sync/rollback) | `argocd.freeze.manage` |

**Additional interactive actions:**
- Sync application
- Hard-refresh
- View diff (live vs desired)
- Rollback to revision

### 🔔 Notifications (1 command)

| Command | Description | Permission |
|---------|-------------|------------|
| `/notify` | Manage notification preferences | None (per-user) |

**Settings:**
- **Severity filter:** Info (all) / Warning+ / Critical only
- **Quiet hours:** 22:00–08:00, 23:00–07:00, 00:00–06:00, or disable
- **Cluster muting:** Mute/unmute alerts per cluster

### 🔐 RBAC (1 command)

| Command | Description | Permission |
|---------|-------------|------------|
| `/rbac` | Manage user roles and permissions | `admin.rbac.manage` |

**Admin actions:**
- List all users with roles
- List all available roles
- Assign role to user
- Revoke role from user
- View user detail (bindings, permissions)

---

## Background Features (no commands)

### 👀 Watcher — Real-time Kubernetes Monitoring

Runs as background informers (only on leader instance). Sends alerts to configured Telegram chats.

| Watcher | What it monitors |
|---------|-----------------|
| **Pod Watcher** | CrashLoopBackOff, OOMKilled, ImagePullBackOff, high restart count |
| **Node Watcher** | NotReady, MemoryPressure, DiskPressure, PID pressure |
| **CronJob Watcher** | Failed jobs, missed schedules |
| **Certificate Watcher** | TLS cert expiry (cert-manager Secrets) |
| **PVC Watcher** | Pending or Lost PersistentVolumeClaims |
| **ArgoCD Watcher** | Application health/sync status changes |
| **Custom Rules** | User-defined alert rules from config |

**Alert deduplication:** Cooldown period per alert key to prevent spam. Mute button on each alert extends cooldown to 1 hour.

### 🛡 AlertManager — Webhook Receiver

HTTP endpoint: `POST /webhook/alertmanager`

- Validates bearer token
- Parses AlertManager payload (Prometheus alerts)
- Formats and sends to configured Telegram chats
- No slash commands — pure webhook integration

### ✅ Approval — Dangerous Operation Gate

- Triggered by other modules (e.g., restart, scale, rollback)
- Configurable required approvals count
- Configurable timeout (auto-expire)
- Inline buttons: Approve / Reject / Cancel
- Audit trail for all decisions

### 📰 Briefing — Scheduled Cluster Reports

- Automatic scheduled reports (cron-based)
- Cluster health summary
- Configurable schedule and timezone

---

## RBAC Role Hierarchy

```
admin > operator > viewer
```

| Role | Capabilities |
|------|-------------|
| **admin** | All permissions. RBAC management, approval decisions, ArgoCD freeze. |
| **operator** | Kubernetes operations (pods, scale, restart, nodes), Helm rollback, ArgoCD sync. |
| **viewer** | Read-only: list pods, logs, events, top, deploys, cronjobs, namespaces, quota, helm list. |

---

## `/help` Output Structure

The `/help` command displays commands grouped by module section:

```
📋 Telekube Commands
━━━━━━━━━━━━━━━━━━━━━━

🤖 Core
  /start — Welcome & cluster selection
  /help — This help message
  /clusters — Switch cluster
  /audit — View audit log

☸️ Kubernetes
  /pods — List pods in a namespace
  /logs <pod> — View pod logs
  /events <pod> — View pod events
  /top — Show pod/node resource usage
  /top nodes — Show node resource usage
  /scale — Scale deployment/statefulset replicas
  /nodes — List and manage cluster nodes
  /quota — Show namespace resource quotas
  /restart <pod> — Restart (delete) a pod
  /namespaces — List all namespaces
  /deploys — List deployments in a namespace
  /cronjobs — List CronJob status

⎈ Helm
  /helm — List and manage Helm releases

🚨 Incident
  /incident — Build incident timeline from K8s events and audit log

🔐 RBAC                          ← only visible to admins
  /rbac — Manage user roles and permissions

🔔 Notifications
  /notify — Manage your notification preferences

🔄 ArgoCD                        ← only if argocd enabled
  /apps — List ArgoCD applications with status
  /dashboard — GitOps status dashboard
  /freeze — Manage deployment freeze

⚙️ Background
  👀 Watcher — Pod/Node/CronJob/Cert/PVC auto-alerts
  🛡 AlertManager — Webhook receiver
  ✅ Approval — Gate for dangerous ops

Use buttons for interactive navigation! 🎛
```

> Commands are filtered by RBAC — users only see commands they have permission for.

---

## Architecture Pattern

```
┌──────────────────────────────────────────┐
│                 Telegram                  │
└──────────────────┬───────────────────────┘
                   │
┌──────────────────▼───────────────────────┐
│            Bot (Middleware)               │
│  Recovery → Auth → RateLimit → Audit     │
└──────────────────┬───────────────────────┘
                   │
┌──────────────────▼───────────────────────┐
│          Module Registry                  │
│  RegisterAll → StartAll → StopAll        │
├──────────────────────────────────────────┤
│ kubernetes │ helm │ incident │ argocd    │
│ watcher    │ notify │ rbac  │ approval  │
│ briefing   │ alertmanager               │
└──────────────────┬───────────────────────┘
                   │
        ┌──────────┼──────────┐
        │          │          │
   ┌────▼────┐ ┌──▼──┐ ┌────▼────┐
   │ Cluster │ │RBAC │ │ Storage │
   │ Manager │ │Engine│ │ (PG/SQ) │
   └────┬────┘ └─────┘ └─────────┘
        │
   ┌────▼────┐
   │  K8s    │
   │ Clients │
   └─────────┘
```
