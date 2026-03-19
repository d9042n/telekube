# Part 7 — Incident & Briefing

> **Objective:** Construct an automated incident timeline from Kubernetes events and audit logs; receive daily cluster health reports on Telegram.

---

## 7.1 Incident Timeline

### Introduction

The Incident module helps you quickly understand **what happened** in the cluster during a specific time frame by aggregating:

- **Kubernetes Events:** Pod crashes, container restarts, scheduler events, OOMKills...
- **Telekube Audit Logs:** User actions via the bot (restart, scale, sync...)

### Activation

```yaml
modules:
  incident:
    enabled: true
```

---

### 7.1.1 `/incident` — Generate Timeline

```
/incident
```

**How to use:**

1. Send `/incident`
2. Select the **namespace** to investigate (or `All Namespaces`)
3. Select the **time window:**
   - `⏱️ Last 15 minutes`
   - `⏱️ Last 30 minutes`
   - `⏱️ Last 1 hour`
   - `⏱️ Last 4 hours`
4. The bot constructs and sends the timeline

**Example output:**

```
🚨 Incident Timeline — production (Last 1 hour)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Cluster: prod-us-east | 13:00 — 14:00 UTC

14:00  💥 api-server-7d8f9b — OOMKilling (container api used 512Mi)
13:58  ⚠️ api-server-7d8f9b — BackOff (Back-off restarting failed container)
13:57  👤 @ops-john — kubernetes.pods.restart api-server-7d8f9b
13:55  ↙️ api-server-7d8f9b — Started
13:54  📦 api-server-7d8f9b — Pulled (Successfully pulled image v1.4.2)
13:50  ⚠️ worker-node-3 — NodeNotReady
13:45  👤 @admin-alice — kubernetes.nodes.drain worker-node-3
13:40  📋 batch-job-28501000 — Scheduled (Pod assigned to worker-node-4)

═══════════════════════════════════════════
Total events: 8
```

**Timeline Legend:**

| Icon | Event Type |
|------|------------|
| 💥 | OOMKill |
| ⚠️ | Warning / NodeNotReady |
| 👤 | User action (audit) |
| ▶️ | Container started |
| 📦 | Image pulled / Pod scheduled |
| 🔁 | BackOff / Restart |
| 📋 | Standard Kubernetes event |

**From the timeline screen:**
- **Refresh** — update the timeline
- **Back** — reselect a namespace

**Minimum role:** `viewer`

> **Limit Notice:** If there are over 30 events, the bot will display "Showing first 30 of X events" and truncate the list. Access Kubernetes directly to view all events.

---

## 7.2 Daily Briefing (Daily Reports)

### Introduction

The Briefing module sends a **consolidated cluster health report** at a scheduled time each day. This helps the team stay informed about the system's status without proactively checking.

### Activation

```yaml
modules:
  briefing:
    enabled: true
    schedule: "0 8 * * *"           # Every day at 8:00 AM
    timezone: "Asia/Ho_Chi_Minh"    # Vietnam Time (UTC+7)
```

---

### 7.2.1 Report Content

The report is automatically sent to all `allowed_chats`:

```
📊 Daily Cluster Briefing
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Tuesday, 03/18/2026 — 08:00 ICT

Cluster: production (prod-us-east)
━━━━━━━━━━━━━━━━

NODES (5/5 healthy)
✅ worker-node-1   Ready   CPU: 45%   RAM: 62%
✅ worker-node-2   Ready   CPU: 38%   RAM: 58%
✅ worker-node-3   Ready   CPU: 71%   RAM: 75%
✅ worker-node-4   Ready   CPU: 29%   RAM: 41%
✅ control-plane   Ready   CPU: 12%   RAM: 35%

PODS
Total: 124 | Running: 121 | Pending: 1 | Failed: 2

⚠️ Pods requiring attention:
  🔴 batch-worker-2  — CrashLoopBackOff (Namespace: batch)
  🔴 report-gen-old  — OOMKilled         (Namespace: reports)
  🟡 cache-warmup    — Pending 8m        (Namespace: backend)

DEPLOYMENTS (48 total)
✅ 46 Available    🟡 2 Degraded

ArgoCD (prod-argocd)
✅ 21 Synced    🟡 1 OutOfSync    🔴 2 Degraded

─────────────────────────────
📅 Yesterday: 3 incidents, 7 user actions
🕐 24h alert count: 5 warnings, 1 critical
```

---

### 7.2.2 Schedule Customization

Examples of common schedules:

```yaml
# 8:00 AM every day
schedule: "0 8 * * *"

# 8:00 AM on weekdays (Monday - Friday)
schedule: "0 8 * * 1-5"

# Twice a day: 8:00 AM and 5:00 PM
schedule: "0 8,17 * * *"

# 9:00 AM every Monday
schedule: "0 9 * * 1"
```

> **Cron syntax:** `<minute> <hour> <day-of-month> <month> <day-of-week>`

---

### 7.2.3 Timezones

The bot automatically converts timestamps to the configured timezone. Use the timezone name defined by the [IANA Time Zone Database](https://www.iana.org/time-zones):

| Region | IANA Name |
|--------|-----------|
| Hanoi / HCMC (UTC+7) | `Asia/Ho_Chi_Minh` |
| Bangkok (UTC+7) | `Asia/Bangkok` |
| Singapore (UTC+8) | `Asia/Singapore` |
| Seoul / Tokyo (UTC+9) | `Asia/Seoul` |
| London (GMT/BST) | `Europe/London` |
| New York (EST/EDT) | `America/New_York` |
| UTC | `UTC` |

---

## 7.3 Summary Table

| Feature | Command / Trigger | Role |
|---------|-------------------|------|
| Incident Timeline | `/incident` | viewer |
| Daily Briefing | Automatic based on schedule | — (all `allowed_chats`) |

---

## Next Steps

- [RBAC & Authorization →](08-rbac.md)
- [Audit Log →](09-audit.md)
