# Part 8 — RBAC & Authorization

> **Objective:** Understand the Role-Based Access Control (RBAC) system, how to assign roles to users, and configure the approval workflow for critical actions.

---

## 8.1 Role System

Telekube includes **5 built-in roles**:

| Role | Access Level |
|------|--------------|
| **viewer** | Read-only — view pods, logs, events, metrics |
| **operator** | viewer + operational actions (restart, scale, sync diff) |
| **admin** | operator + administration (audit, RBAC, freeze, sync, rollback) |
| **on-call** | operator + rollback + drain (auto-expires) |
| **super-admin** | Full access — unrestricted |

---

## 8.2 Detailed Permissions

### Viewer

| Permission | Description |
|------------|-------------|
| Pod list, get | View pod list, pod details |
| Pod logs | View logs |
| Pod events | View K8s events |
| Metrics view | View CPU/RAM (`/top`) |
| Node view | View node list |
| Quota view | View resource quotas |
| ArgoCD apps list/view | View app list and details |

### Operator (in addition to viewer)

| Permission | Description |
|------------|-------------|
| Pod restart | Restart pod |
| Deployment scale | Scale deployment/statefulset |
| ArgoCD diff | View diff before sync |
| Helm releases list | View Helm release list |

### Admin (in addition to operator)

| Permission | Description |
|------------|-------------|
| Pod delete | Delete pod |
| Node cordon | Cordon/uncordon node |
| Node drain | Drain node |
| ArgoCD sync | Force sync ArgoCD app |
| ArgoCD rollback | Rollback ArgoCD app |
| ArgoCD freeze manage | Create/remove deployment freeze |
| Helm rollback | Rollback Helm release |
| Admin users manage | User management |
| Admin RBAC manage | RBAC management |
| Admin audit view | View audit log |

### On-Call (in addition to operator)

| Permission | Description |
|------------|-------------|
| ArgoCD rollback | Emergency rollback during incidents |
| Node drain | Emergency workload eviction |

> **Note:** The `on-call` role is designed for on-call engineers during emergencies. It can be configured to auto-expire after a set duration.

---

## 8.3 Bot Admin

Users listed in `telegram.admin_ids` in the config file **always have admin rights** and cannot be demoted via the bot.

```yaml
telegram:
  admin_ids: [123456789, 987654321]
```

This serves as a bootstrap mechanism — used to grant permissions to other users.

---

## 8.4 Assigning User Roles

### View Current Roles

```
/rbac
```

Bot admins can use the `/rbac` command to:
- View the user list and their roles
- Assign roles to users

### Assign a Role

**Minimum role:** `admin`

Workflow in Telegram:

1. Admin sends `/rbac`
2. Select the user to manage
3. Select the new role
4. Confirm

Or add manually to the database (for bootstrapping):

```sql
-- SQLite
INSERT INTO users (telegram_id, username, role, created_at, updated_at)
VALUES (987654321, 'john_doe', 'operator', datetime('now'), datetime('now'));
```

### Self-Registration (if configured)

New users automatically receive the `viewer` role based on the configuration:

```yaml
rbac:
  default_role: viewer  # or "operator" for more open access
```

---

## 8.5 Approval Workflow

When the approval module is enabled, certain sensitive actions **require approval** before execution.

### Activation

```yaml
modules:
  approval:
    enabled: true
```

### How it works

```
Operator requests production sync    
         ↓
System creates ApprovalRequest (ID: req-01JXYZ)
         ↓
Notification sent to admin chat:
  ┌──────────────────────────────────┐
  │ 🔔 Approval Request              │
  │                                  │
  │ From: @john_operator             │
  │ Action: argocd.apps.sync         │
  │ App: backend-api (production)    │
  │ Time: 14:00 UTC                  │
  │ Expires in: 30 minutes           │
  │                                  │
  │ [✅ Approve] [❌ Reject]          │
  └──────────────────────────────────┘
         ↓
Admin clicks Approve / Reject
         ↓
If Approved → System automatically executes the sync
If Rejected → Operator receives a rejection notice
```

### Rules

Approval rules are configured in the following format:

```go
// Programmatic configuration (in code/config)
Rules: []Rule{
    {
        Action:            "argocd.apps.sync",
        Clusters:          []string{"production"},
        RequiredApprovals: 1,
        ApproverRoles:     []string{"admin", "super-admin"},
    },
    {
        Action:            "argocd.apps.rollback",
        Clusters:          []string{"*"},   // All clusters
        RequiredApprovals: 1,
        ApproverRoles:     []string{"admin"},
    },
}
```

### View Pending Requests

Pending approval requests appear as interactive messages in the admin chat with **Approve / Reject / Cancel** inline buttons. There is no `/approve` slash command — approvals are handled entirely via inline buttons when a request is submitted.

**Minimum role to approve/reject:** `admin`

### Approval Statuses

| Status | Description |
|--------|-------------|
| `pending` | Waiting for approval |
| `approved` | Approved, action is being executed |
| `rejected` | Denied |
| `expired` | Expired (defaults to 30 mins) |
| `cancelled` | Cancelled by the requester |

### Self-Approval (Not Allowed)

The system prevents requesters from approving their own requests:

```
❌ You cannot approve your own request.
Please ask another admin for approval.
```

---

## 8.6 On-Call Role

The `on-call` role is suitable for engineers handling emergencies during nights or weekends:

**Permissions:**
- All `operator` permissions
- Rollback ArgoCD app (no approval required)
- Drain node (in emergencies)

**Typical Use Case:**

```
1. Production incident at 2 AM
2. On-call engineer urgently needs to rollback
3. Admin temporarily assigns the "on-call" role to the engineer via /rbac
4. Engineer executes the rollback autonomously
5. Once resolved, admin revokes the "on-call" role
```

---

## 8.7 Complete Permission Matrix

| Action | viewer | operator | on-call | admin | super-admin |
|--------|:------:|:--------:|:-------:|:-----:|:-----------:|
| View pods | ✅ | ✅ | ✅ | ✅ | ✅ |
| View logs | ✅ | ✅ | ✅ | ✅ | ✅ |
| View metrics (`/top`) | ✅ | ✅ | ✅ | ✅ | ✅ |
| View nodes | ✅ | ✅ | ✅ | ✅ | ✅ |
| View ArgoCD apps | ✅ | ✅ | ✅ | ✅ | ✅ |
| Restart pod | — | ✅ | ✅ | ✅ | ✅ |
| Scale deployment | — | ✅ | ✅ | ✅ | ✅ |
| ArgoCD diff | — | ✅ | ✅ | ✅ | ✅ |
| View Helm releases | — | ✅ | ✅ | ✅ | ✅ |
| Delete pod | — | — | — | ✅ | ✅ |
| Cordon/Uncordon node | — | — | — | ✅ | ✅ |
| Drain node | — | — | ✅ | ✅ | ✅ |
| ArgoCD sync | — | — | — | ✅ | ✅ |
| ArgoCD rollback | — | — | ✅ | ✅ | ✅ |
| Deployment freeze | — | — | — | ✅ | ✅ |
| Helm rollback | — | — | — | ✅ | ✅ |
| Approve requests | — | — | — | ✅ | ✅ |
| View audit log | — | — | — | ✅ | ✅ |
| Manage RBAC | — | — | — | ✅ | ✅ |

---

## Next Steps

- [Audit Log →](09-audit.md)
- [Command Reference →](10-commands.md)
