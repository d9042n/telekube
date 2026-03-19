# Part 9 — Audit Log

> **Objective:** View and understand the action log (audit log) — who did what, when, and on which cluster.

---

## 9.1 Introduction

Telekube automatically records all **operational** actions executed via the bot into the Audit Log. Each entry includes:

- **Actor** (Telegram username + ID)
- **Action** (restart, scale, sync, rollback...)
- **Target Resource** (pod name, deployment, app...)
- **Cluster and Namespace**
- **Timestamp** (UTC)
- **Result** (success / failed)

The audit log helps answer: *"Who restarted this pod? Who scaled that deployment? Who synced ArgoCD at 2 AM?"*

---

## 9.2 `/audit` — View Audit Log

```
/audit
```

**Minimum role:** `admin`

### How to use

1. Send `/audit`
2. The bot returns the 20 most recent actions (newest on top):

```
📋 Audit Log — 20 entries
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

14:05 UTC  @john_ops       kubernetes.pods.restart
           api-server-7d8f9b    prod / backend    ✅

13:57 UTC  @alice_admin     argocd.apps.sync
           backend-api          prod-argocd       ✅

13:45 UTC  @alice_admin     kubernetes.nodes.drain
           worker-node-3        production        ✅

13:30 UTC  @bob_dev         kubernetes.deployments.scale
           frontend (3→5)       prod / frontend   ✅

12:10 UTC  @john_ops       kubernetes.pods.restart
           worker-svc-abc       prod / batch      ❌ (forbidden)

...
```

**Result Legend:**
- ✅ `success` — operation successful
- ❌ `failed` — operation failed (reason detailed internally)
- 🚫 `forbidden` — insufficient permissions

---

## 9.3 Using Audit Log within Incidents

The audit log is integrated into the **Incident Timeline**. When you use `/incident`, user actions (👤) appear interleaved with Kubernetes events:

```
14:00  💥 api-server — OOMKilling
13:58  ⚠️ api-server — BackOff restart
13:57  👤 @ops-john — kubernetes.pods.restart api-server-7d8f9b   ← from audit log
13:55  ▶️ api-server — Started
```

This helps analyze **cause and effect** — correlating user behavior with cluster events.

---

## 9.4 Tracked Actions

| Action | Description |
|--------|-------------|
| `kubernetes.pods.restart` | Restart pod |
| `kubernetes.pods.delete` | Delete pod |
| `kubernetes.deployments.scale` | Scale deployment |
| `kubernetes.nodes.cordon` | Cordon node |
| `kubernetes.nodes.uncordon` | Uncordon node |
| `kubernetes.nodes.drain` | Drain node |
| `argocd.apps.sync` | Sync ArgoCD app |
| `argocd.apps.rollback` | Rollback ArgoCD app |
| `argocd.freeze.create` | Create deployment freeze |
| `argocd.freeze.thaw` | Remove deployment freeze |
| `helm.releases.rollback` | Rollback Helm release |
| `admin.rbac.assign` | Assign user role |
| `admin.rbac.revoke` | Revoke user role |

---

## 9.5 Storage and Pagination

- The audit log is stored in the database (SQLite **or** PostgreSQL)
- Shows the top 20 latest entries by default
- Data is not automatically purged — manual management is required if size becomes an issue
- For advanced querying (filtering by user, action, time), access the database directly

### Direct Database Queries

**SQLite:**

```bash
sqlite3 telekube.db

-- 50 actions by @john_ops
SELECT occurred_at, username, action, resource, cluster, namespace, status
FROM audit_log
WHERE username = 'john_ops'
ORDER BY occurred_at DESC
LIMIT 50;

-- All sync actions on production today
SELECT occurred_at, username, action, resource, status
FROM audit_log
WHERE action LIKE 'argocd.apps.sync'
  AND cluster = 'production'
  AND date(occurred_at) = date('now')
ORDER BY occurred_at DESC;
```

**PostgreSQL:**

```sql
-- Actions within the last 24 hours
SELECT occurred_at, username, action, resource, cluster, namespace, status
FROM audit_log
WHERE occurred_at >= NOW() - INTERVAL '24 hours'
ORDER BY occurred_at DESC;
```

---

## 9.6 Summary Table

| Feature | Description | Role |
|---------|-------------|------|
| `/audit` | View the 20 most recent actions | admin |
| Auditing in `/incident` | Combined K8s events + audit timeline | viewer |
| Database Queries | Advanced search capabilities | System Admin |

---

## Next Steps

- [Full Command Reference →](10-commands.md)
- [Back to Table of Contents →](README.md)
