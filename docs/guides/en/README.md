# Telekube User Guide (English)

> **Telekube** — Kubernetes & ArgoCD Command Center via Telegram

---

## Table of Contents

| # | Document | Description |
|---|----------|-------------|
| 1 | [Installation & Getting Started](01-installation.md) | Installation, Telegram bot creation, initial configuration |
| 2 | [Configuration](02-configuration.md) | Configuration files, environment variables, options |
| 3 | [Kubernetes](03-kubernetes.md) | Manage pods, nodes, deployments, logs, metrics |
| 4 | [ArgoCD & GitOps](04-argocd.md) | Sync, rollback, dashboard, deployment freeze |
| 5 | [Helm](05-helm.md) | View and rollback Helm releases |
| 6 | [Monitoring & Alerting](06-watcher.md) | Real-time alerts: OOM, node down, cert expiry, PVC |
| 7 | [Incident & Briefing](07-incident-briefing.md) | Incident timeline, daily health reports |
| 8 | [RBAC & Authorization](08-rbac.md) | User roles, authorization, approval workflow |
| 9 | [Audit Log](09-audit.md) | View action logs |
| 10 | [Command Reference](10-commands.md) | Full list of all commands |

---

## Quick Overview

Telekube turns Telegram into a powerful Command Center to:

- **Manage Kubernetes** — view pods, logs, events, scale, restart, node management
- **GitOps with ArgoCD** — sync apps, rollback, view diffs, deployment freeze
- **Proactive Monitoring** — real-time alerts for pod crash, OOMKill, node down, cert expiry
- **Manage Helm** — list releases, view history, quick rollback
- **Incident Response** — automated incident timeline from K8s events and audit logs
- **Daily Reports** — cluster health summary on Telegram every morning
- **Administration** — RBAC, approval workflow, full audit log

---

_Last updated: 2026-03-18_
