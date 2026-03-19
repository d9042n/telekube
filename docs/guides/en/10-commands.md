# Part 10 — Command Reference

> Complete list of all Telekube Telegram commands grouped by functionality.

---

## 10.1 General Commands

| Command | Description | Role |
|---------|-------------|------|
| `/start` | Welcome screen + cluster selection | All |
| `/help` | List of available commands | All |
| `/clusters` | Switch active cluster | All |

---

## 10.2 Kubernetes — Read

| Command | Description | Role |
|---------|-------------|------|
| `/pods` | Pod list (select namespace via button) | viewer |
| `/pods <namespace>` | Pod list in a specific namespace | viewer |
| `/logs <pod>` | View the last 100 log lines of a pod | viewer |
| `/events <pod>` | View Kubernetes events for a pod | viewer |
| `/nodes` | Node list and statuses | viewer |
| `/top` | Pod CPU/RAM metrics (select namespace) | viewer |
| `/top nodes` | Node CPU/RAM metrics | viewer |
| `/quota` | Resource quotas per namespace | viewer |
| `/namespaces` | List all namespaces with status | viewer |
| `/deploys [ns]` | List deployments in a namespace | viewer |
| `/cronjobs [ns]` | List CronJob status | viewer |

---

## 10.3 Kubernetes — Operations

| Command / Action | Description | Role |
|------------------|-------------|------|
| `/restart <pod>` | Restart a pod (delete + controller recreates) | operator |
| Restart pod (from detail) | Restart pod via button | operator |
| `/scale` | Scale Deployment/StatefulSet | operator |
| Cordon node (from detail) | Restrict new pod scheduling | admin |
| Uncordon node (from detail) | Re-enable pod scheduling | admin |
| Drain node (from detail) | Evict workloads, prep node for maintenance | admin |
| Delete pod (from detail) | Delete pod permanently | admin |

---

## 10.4 ArgoCD

| Command / Action | Description | Role |
|------------------|-------------|------|
| `/apps` | List ArgoCD apps and their status | viewer |
| `/dashboard` | Consolidated GitOps dashboard | viewer |
| View diff (from app detail) | Diff preview before sync | operator |
| Sync app (from app detail) | Force sync with Git | admin |
| Rollback app (from app detail) | Revert to previous revision | admin |
| `/freeze` | Create / remove Deployment Freeze | admin |

---

## 10.5 Helm

| Command / Action | Description | Role |
|------------------|-------------|------|
| `/helm` | Helm release list (select namespace) | operator |
| View release details | Chart, version, revision history | operator |
| Rollback release | Rollback to a specific Helm revision | admin |

---

## 10.6 Monitoring & Alerting

| Feature | Trigger | Role |
|---------|---------|------|
| Pod crash alert | Automatic (watcher) | — |
| Node down alert | Automatic (watcher) | — |
| CronJob failed alert | Automatic (watcher) | — |
| Cert expiry alert | Automatic (watcher) | — |
| PVC usage alert | Automatic (watcher) | — |
| Custom alert rules | Automatic (evaluator, config-driven) | — |
| AlertManager webhook | Automatic (Prometheus) | — |
| Mute alert 1 hour | Click button in the alert | viewer |

---

## 10.7 Incident & Reporting

| Command | Description | Role |
|---------|-------------|------|
| `/incident` | Generate incident timeline | viewer |
| Daily Briefing | Automatic via schedule (cron) | — |

---

## 10.8 Administration

| Command | Description | Role |
|---------|-------------|------|
| `/audit` | View the 20 most recent operational actions | admin |
| `/rbac` | Manage user roles and bindings | admin |
| `/notify` | Manage notification preferences (severity, quiet hours, muting) | all |

> `/approve` is not a slash command — approval actions are triggered via inline buttons when an approval request is sent.

---

## 10.9 Quick Comparison Matrix

```
VIEWER           OPERATOR              ADMIN
━━━━━━━━━━━━━━   ━━━━━━━━━━━━━━━━━━   ━━━━━━━━━━━━━━━━━━━━━━━━
/start           /start               /start
/help            /help                /help
/clusters        /clusters            /clusters
/pods            /pods                /pods
/logs            /logs                /logs
/events          /events              /events
/nodes           /nodes               /nodes
/top             /top                 /top
/quota           /quota               /quota
/apps            /apps                /apps
/dashboard       /dashboard           /dashboard
/incident        /incident            /incident
/notify          /notify              /notify
                 Restart pod          Restart pod
                 /scale               /scale
                 /helm                Cordon/Uncordon node
                                      Drain node
                                      Delete pod
                                      Sync ArgoCD
                                      Rollback ArgoCD
                                      Helm rollback
                                      /freeze
                                      /audit
                                      /rbac
                                      Approve requests
```

---

## 10.10 Shortcuts and Inline Buttons

Most Telekube features utilize **inline keyboard buttons** rather than requiring users to memorize complex command syntaxes.

**Typical Workflow:**

```
/pods
  → Select namespace [button]
    → Pod list
      → Click pod name [button]  
        → Pod Detail
          → [Logs] [Events] [Restart] [Back]
```

```
/nodes
  → Node list
    → Click node name [button]
      → Node Detail
        → [Cordon] [Uncordon] [Drain] [Top Pods] [Back]
```

```
/apps
  → Select instance [button] (if multiple instances exist)
    → App list
      → Click app name [button]
        → App Detail
          → [Sync] [Diff] [Rollback] [Back]
```

---

## 10.11 Common Error Troubleshooting

| Error | Cause | Solution |
|-------|-------|----------|
| `🚫 Insufficient permissions` | Lack of permissions | Contact admin for access |
| `No clusters configured` | Clusters not set | Check `configs/config.yaml` |
| `metrics-server not available` | Metrics server missing | Install metrics-server on the cluster |
| `ArgoCD instance not found` | ArgoCD module off or unconfigured | Set `modules.argocd.enabled: true` and define instance |
| `Deployment freeze active` | Freeze is currently active | Wait until it expires or contact admin |
| `request expired` | Request passed the 30m window | Submit a new request |
| `Rate limit exceeded` | Command spamming | Wait 1 minute and retry |

---

## Additional References

- [Installation & Getting Started](01-installation.md)
- [Configuration](02-configuration.md)
- [RBAC & Authorization](08-rbac.md)
- [Back to Table of Contents](README.md)
