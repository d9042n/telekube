# Part 5 — Helm

> **Objective:** View the Helm release list, inspect details, and rollback to a previous revision directly from Telegram.

---

## 5.1 Requirements & Activation

```yaml
modules:
  helm:
    enabled: true
```

The Helm module utilizes the Kubernetes REST config to access Helm secrets — no need to install the Helm CLI on the server running Telekube.

---

## 5.2 `/helm` — Helm Release List

```
/helm
```

### How to use

1. Send `/helm`
2. The bot prompts to select a namespace:
   - `[All Namespaces]` — view all namespaces
   - `production`
   - `staging`
   - `kube-system`
3. The bot returns a list of releases with info:

```
⎈ Helm Releases — production (cluster: my-cluster)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ backend-api         1.4.2    deployed   Rev 8   2h ago
✅ frontend            2.1.0    deployed   Rev 12  1d ago
🔴 worker-service      3.0.1    failed     Rev 5   30m ago
🟡 batch-job           1.0.0    pending    Rev 2   5m ago
```

**Release Status Legend:**

| Icon | Helm Status |
|------|-------------|
| ✅ | deployed — running normally |
| 🔴 | failed — deployment failed |
| 🟡 | pending-install / pending-upgrade / pending-rollback |
| ⚪ | Other statuses |

4. Click a release name to view details

**Minimum role:** `operator` (to view list)

---

## 5.3 Release Details

The details screen shows:

```
⎈ backend-api (production)
━━━━━━━━━━━━━━━━━━━━━━
Chart:    backend-chart-0.9.3
App:      1.4.2
Status:   deployed
Revision: 8
Updated:  2026-03-18 06:00 UTC

History:
  Rev 8  — 1.4.2    (current)
  Rev 7  — 1.4.1
  Rev 6  — 1.4.0
  Rev 5  — 1.3.9
  Rev 4  — 1.3.8
```

From the details screen you can:
- **Rollback** — choose a previous revision to revert to
- **Back** — return to the namespace list

---

## 5.4 Rollback Helm Release

Rollback reverts a release to a specific revision:

1. Click on a release → **Rollback**
2. The bot lists the revision history (up to the last 10):
   ```
   Select revision to rollback:
   ─────────────────────────
   Rev 7 — 1.4.1
   Rev 6 — 1.4.0
   Rev 5 — 1.3.9
   ```
3. Select the revision you want to rollback to
4. The bot confirms and executes the rollback (5-minute timeout)
5. The bot announces the result:
   ```
   ✅ Rollback successful! backend-api is now at Rev 9 (rolled back to 1.4.1)
   ```

> **Note:** Every rollback generates a new revision. For example, rolling back from Rev 8 to Rev 7 creates Rev 9 with the chart matching Rev 7.

**Minimum role:** `admin` (for rollback)

---

## 5.5 Refresh

Click the **Refresh** button to update the release list (e.g. if a new release was deployed).

---

## 5.6 Helm Command Summary

| Action | Description | Role |
|--------|-------------|------|
| `/helm` | View release list | operator |
| View release details | Chart, version, history | operator |
| Rollback release | Revert to an earlier revision | admin |

---

## 5.7 Important Notes

- Telekube only supports **viewing** and **rolling back** — it does not support installing or upgrading releases via Telegram.
- To deploy a new Helm chart or upgrade, use ArgoCD (GitOps) or the Helm CLI.
- The history displays up to the **last 10 revisions**.

---

## Next Steps

- [Monitoring & Alerting →](06-watcher.md)
- [Incident & Briefing →](07-incident-briefing.md)
