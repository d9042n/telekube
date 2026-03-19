# Part 6 — Monitoring & Alerting (Watcher)

> **Objective:** Receive real-time alerts on Telegram for cluster incidents: pod crashes, OOMKill, node down, expiring certs, full PVCs, failed CronJobs.

---

## 6.1 Requirements & Activation

```yaml
modules:
  watcher:
    enabled: true

telegram:
  # Chats to receive alerts (in addition to admins)
  allowed_chats: [-1001234567890]  # ID of the on-call group chat
```

> **Important:** The Watcher only sends alerts to the chats listed in `allowed_chats`. Admin chats will always receive them.

---

## 6.2 Watcher Types

### 6.2.1 Pod Watcher

**Monitors:** Abnormal pod status changes

**Alerts when:**
- Pod enters `CrashLoopBackOff` state
- Pod is `OOMKilled` (out of memory)
- Pod cannot start (`ImagePullBackOff`, `ErrImagePull`)
- Pod is `Pending` for too long (> 5 minutes)

**Alert Example:**

```
🔴 Pod Crash Alert

Cluster: production
Namespace: backend
Pod: api-server-7d8f9b-xvz4k

Status: CrashLoopBackOff
Restarts: 12
Reason: OOMKilled — used 512Mi, limit 256Mi

[Logs] [Events] [Mute 1h]
```

**Action buttons:**
- **Logs** — view pod logs directly
- **Events** — view K8s events
- **Mute 1h** — mute alerts for this pod for 1 hour

---

### 6.2.2 Node Watcher

**Monitors:** Node statuses in the cluster

**Alerts when:**
- Node transitions to `NotReady`
- Node is under `MemoryPressure`, `DiskPressure`, or `PIDPressure`
- Node disconnects (`Unknown`)

**Alert Example:**

```
⚡ Node Alert

Cluster: production
Node: worker-node-3

Status: NotReady
Condition: MemoryPressure
Duration: 2m 30s

[Node Details] [Mute 1h]
```

---

### 6.2.3 CronJob Watcher

**Monitors:** CronJob execution results

**Alerts when:**
- A CronJob fails (exit code ≠ 0)
- CronJob misses a scheduled run (delayed)
- Consecutive failures exceed the threshold

**Alert Example:**

```
⏰ CronJob Failed

Cluster: production
Namespace: batch
CronJob: nightly-report

Job: nightly-report-28501200
Status: Failed
Started: 02:00:00 UTC
Duration: 4m 23s

[Job Logs] [Mute 1h]
```

> **Excluded Namespace:** `kube-system` is excluded by default to prevent alert noise from system cronjobs.

---

### 6.2.4 Certificate Watcher

**Monitors:** TLS certificates stored in Kubernetes Secrets (type `kubernetes.io/tls`)

**Alerts when:**
- Cert expires in **30 days**
- Cert expires in **7 days** (critical warning)
- Cert has expired

**Alert Example:**

```
🔐 Certificate Expiry Warning

Cluster: production
Namespace: ingress-nginx
Secret: api-tls-cert

Common Name: api.example.com
Expires: 2026-04-01 (in 14 days)
Issuer: Let's Encrypt Authority X3

[Mute 7d]
```

> **Excluded Namespace:** `kube-system` is excluded by default.

---

### 6.2.5 PVC Watcher

**Monitors:** PersistentVolumeClaim usage capacity

**Alerts when:**
- PVC usage > **85%** capacity
- PVC usage > **95%** capacity (critical warning)
- PVC is in `Pending` state (unbound)

**Alert Example:**

```
💾 PVC Usage Alert

Cluster: production
Namespace: database
PVC: postgres-data-pvc

Used: 85.2% (8.5Gi / 10Gi)
Available: 1.5Gi

[Mute 1h]
```

---

### 6.2.6 ArgoCD Watcher

**Monitors:** Real-time status of ArgoCD apps

**Alerts when:**
- App becomes `Degraded`
- App is `OutOfSync` for a prolonged time (cannot sync)
- Sync fails

**Alert Example:**

```
🚀 ArgoCD App Alert

Instance: prod-argocd
App: backend-api

Health: Degraded
Sync: OutOfSync
Message: Deployment "backend-api" has unavailable replicas

[View App] [Sync] [Mute 1h]
```

---

## 6.3 Custom Alert Rules

In addition to built-in watchers, you can define **custom alert rules** in the configuration file. These are evaluated periodically (every 30 seconds).

### Supported Condition Types

| Type | Description |
|------|-------------|
| `pod_restart_count` | Alert when container restarts exceed threshold |
| `pod_pending_duration` | Alert when pods stay Pending beyond time window |
| `namespace_quota_percentage` | Alert when resource quota exceeds percentage |
| `deployment_unavailable` | Alert when deployments have zero available replicas |

### Configuration Example

```yaml
watcher:
  custom_rules:
    - name: "high-restarts"
      description: "Container restart count too high"
      severity: "warning"
      condition:
        type: "pod_restart_count"
        threshold: 5
      scope:
        clusters: ["*"]
        namespaces: ["production"]

    - name: "deployment-down"
      description: "Deployment has no available replicas"
      severity: "critical"
      condition:
        type: "deployment_unavailable"
      scope:
        clusters: ["production"]
        namespaces: ["*"]
```

### Features

- **Scoped evaluation**: Limit rules to specific clusters and/or namespaces
- **Deduplication**: 10-minute cooldown to prevent repeated alerts
- **Audit logging**: All triggered custom alerts are recorded in the audit log
- **Custom routing**: Route alerts to specific chats

---

## 6.4 Anti-Spam & Deduplication

The Watcher automatically prevents spamming repeated alerts:

- **Cooldown:** Each alert has a 5-minute cooldown — if the same issue persists, it will not be resent.
- **Deduplication:** Identical alerts (same pod/node) are aggregated.
- **Mute:** Users can click **Mute 1h** to suppress alerts for a specific resource for 1 hour.

---

## 6.5 AlertManager Webhook

In addition to built-in watchers, Telekube supports receiving alerts from the **Prometheus AlertManager** via webhook:

### Activation

```yaml
modules:
  alertmanager:
    enabled: true

server:
  port: 8080
```

### AlertManager Configuration

Add the webhook receiver to your Prometheus AlertManager config file:

```yaml
# alertmanager.yml
receivers:
  - name: 'telekube'
    webhook_configs:
      - url: 'http://telekube:8080/api/v1/alertmanager/webhook'
        send_resolved: true

route:
  receiver: 'telekube'
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
```

### AlertManager Alert Example

```
🚨 Alert: HighMemoryUsage

Cluster: production
Severity: critical

Labels:
  alertname: HighMemoryUsage
  namespace: backend
  pod: api-server-7d8f9b

Annotations:
  summary: Pod memory usage > 90%
  description: Pod api-server-7d8f9b is using 92% of memory limit

Status: firing
Started: 2026-03-18 14:00 UTC
```

---

## 6.6 Advanced Watcher Configuration

Watchers are configured automatically when the module is enabled. If you need to customize the exclusion list:

```go
// Example: exclude additional namespaces
m.certWatcher = NewCertWatcher(clusters, notifier, audit, cfg, CertWatcherConfig{
    ExcludeNamespaces: []string{"kube-system", "monitoring", "logging"},
}, logger)
```

> Customizing the exclusion list currently requires code modification. Configuration file support will be added in a future release.

---

## 6.7 Watcher Summary

| Watcher | Monitors | Alert Thresholds |
|---------|----------|------------------|
| Pod Watcher | Pod status | CrashLoopBackOff, OOMKill, Pending > 5min |
| Node Watcher | Node status | NotReady, MemoryPressure, DiskPressure |
| CronJob Watcher | CronJob execution | Job Failed, Miss Schedule |
| Cert Watcher | TLS cert expiry | < 30 days, < 7 days |
| PVC Watcher | PVC usage capacity | > 85%, > 95% |
| ArgoCD Watcher | ArgoCD app status | Degraded, prolonged OutOfSync |
| Custom Rules | Config-driven conditions | Restart count, pending, quota, unavailable |

---

## Next Steps

- [Incident & Briefing →](07-incident-briefing.md)
- [RBAC & Authorization →](08-rbac.md)
