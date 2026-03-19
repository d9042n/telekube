# Part 4 — ArgoCD & GitOps

> **Objective:** Manage ArgoCD applications, perform syncs, rollbacks, view the dashboard, and control deployments via Deployment Freeze.

---

## 4.1 Requirements & Activation

### Enable module

```yaml
modules:
  argocd:
    enabled: true

argocd:
  insecure: false
  timeout: 30s
  instances:
    - name: "prod-argocd"
      url: "https://argocd.example.com"
      auth:
        type: "token"
        token: "${ARGOCD_TOKEN}"
      clusters:
        - "production"
```

### Obtain ArgoCD Token

```bash
# Login via ArgoCD CLI
argocd login argocd.example.com --username admin

# Generate token for Telekube
argocd account generate-token --account telekube
```

Save the token to an environment variable:

```bash
export ARGOCD_TOKEN="eyJhbGciOiJIUzI1NiI..."
```

---

## 4.2 `/apps` — Application List

```
/apps
```

### How to use

1. Send `/apps`
2. If there are multiple ArgoCD instances, the bot asks you to select one
3. The bot lists applications with their statuses:

```
✅ backend-api        Synced    Healthy
🟡 frontend           OutOfSync Healthy
🔴 worker-service     Synced    Degraded
⚪ batch-job          Synced    Progressing
```

**Status Legend:**

| Icon | Sync Status | Health Status |
|------|-------------|---------------|
| ✅ | Synced | Healthy |
| 🟡 | OutOfSync | — |
| 🔴 | — | Degraded/Missing |
| ⚪ | — | Progressing/Unknown |

4. Click on an application's name to view details

### Application Details

The details screen displays:
- Current **Git repo** and **branch/revision**
- **Helm chart** (if using Helm)
- List of **resources** (Deployment, Service, Ingress...) with their individual statuses
- Recent sync history

From the details screen, you can:
- **Sync** — synchronize with Git
- **Rollback** — revert to a previous revision
- **Back** — return to the list

**Minimum role:** `viewer`

---

## 4.3 Sync Application

### Standard Sync

1. Click on an application → **Sync**
2. The bot displays a sync options menu:
   - **Sync Now** — standard sync
   - **Sync + Prune** — sync and delete resources removed from Git
   - **Force Sync** — force overwrite local state
3. Confirm
4. The bot performs the sync and announces the result

> **Note:** The Sync action requires the `admin` role by default. See [ArgoCD Authorization](08-rbac.md#argocd).

**Minimum role:** `admin`

### Diff Before Sync

Before syncing, an `operator` can preview the diff:

1. Click on an application
2. Click **Diff** (applicable if the app is OutOfSync)
3. The bot displays what will change upon syncing

**Minimum role:** `operator`

---

## 4.4 Rollback Application

Rollback reverts the application to a previous revision in the ArgoCD history:

1. Click on an application → **Rollback**
2. The bot lists the revisions from the last 10 syncs:
   ```
   Rev 42 — commit abc1234 (2h ago) — Healthy
   Rev 41 — commit def5678 (1d ago) — Healthy
   Rev 40 — commit ghi9012 (3d ago) — Degraded
   ```
3. Select the revision you want to rollback to
4. Confirm
5. The bot executes the rollback

**Minimum role:** `admin`

> **Emergency:** In production incidents, an on-call engineer with the `on-call` role can also perform rollbacks.

---

## 4.5 `/dashboard` — GitOps Dashboard

```
/dashboard
```

The consolidated dashboard displays the status of all ArgoCD applications:

```
GitOps Dashboard — prod-argocd
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Total:      24 apps
Healthy:    21
Degraded:    2
OutOfSync:   1

✅ 21 Synced & Healthy
🟡  1 Out of Sync
🔴  2 Degraded
```

From the dashboard you can:
- **Refresh** — update statuses
- **Filter Out of Sync** — view only OutOfSync apps
- **Filter Degraded** — view only failing apps

**Minimum role:** `viewer`

---

## 4.6 `/freeze` — Deployment Freeze

Deployment Freeze is a feature that **blocks all sync and rollback actions** for a specified duration. Use cases include:

- Maintenance windows
- Before/during major events (Black Friday, product launches...)
- Investigating production incidents

---

### Create a Freeze

1. Send `/freeze`
2. The bot displays a menu:
   - **Create new freeze**
   - **View freeze history**
   - **Remove active freeze**
3. Select **Create new freeze**
4. Select the **scope**:
   - `All applications` — block all sync/rollback everywhere
   - `Specific namespace` — block only within the chosen namespace
5. Select the **duration**:
   - 1 hour / 2 hours / 4 hours / 8 hours / 24 hours
6. Confirm — The freeze takes effect immediately

**Result:** When a user attempts to sync/rollback during a freeze:

```
🧊 Deployment Freeze is active!

Scope: All applications
Time: 14:00 → 18:00 (2h30m remaining)
Reason: Maintenance window

Contact an admin for emergency deployments.
```

**Minimum role:** `admin`

---

### Remove Freeze (Thaw)

1. Send `/freeze`
2. Select **Remove current freeze** (Thaw)
3. Confirm

---

### View Freeze History

1. Send `/freeze`
2. Select **History**
3. The bot displays the list of freezes over the last 30 days

---

## 4.7 Sync/Rollback with Approval Workflow

When the `approval` module is enabled and ArgoCD approval configuration is set, sync/rollback actions on production clusters will require approval:

1. An `operator` requests a sync
2. The system creates an approval request and notifies admins
3. Admin clicks **Approve** or **Reject**
4. If approved, the sync is executed automatically

Learn more in [Part 8 — RBAC & Approval](08-rbac.md).

---

## 4.8 ArgoCD Command Summary

| Command / Action | Description | Role |
|------------------|-------------|------|
| `/apps` | Application list | viewer |
| `/dashboard` | GitOps dashboard | viewer |
| App Diff | Preview deployment changes | operator |
| Sync App | Synchronize with Git | admin |
| Rollback App | Revert to previous revision | admin |
| `/freeze` | Create/remove deployment freeze | admin |

---

## Next Steps

- [Helm Management →](05-helm.md)
- [Monitoring & Alerting →](06-watcher.md)
- [RBAC & Approval →](08-rbac.md)
