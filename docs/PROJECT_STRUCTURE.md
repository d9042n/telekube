# Telekube — Project Structure

> A modular Telegram bot for Kubernetes cluster management, ArgoCD GitOps, Helm releases, incident response, and real-time monitoring.

---

## Top-Level Layout

```
telekube/
├── cmd/telekube/           # Application entrypoint
├── configs/                # Configuration files (YAML)
├── deploy/                 # Deployment manifests
│   ├── docker/             # Dockerfile
│   └── helm/               # Helm chart for K8s deployment
├── docs/                   # Documentation
│   ├── documentation/      # ADRs, architecture decisions
│   ├── features/           # Feature descriptions
│   └── guides/             # User guides (en/vi)
├── internal/               # Private application code
│   ├── app/                # Application lifecycle (DI wiring, run loop)
│   ├── audit/              # Audit logging interface
│   ├── bot/                # Telegram bot core (handlers, middleware, keyboard)
│   ├── cluster/            # Kubernetes cluster manager
│   ├── config/             # Configuration structs (Viper)
│   ├── entity/             # Domain models
│   ├── leader/             # Leader election (Redis-based)
│   ├── module/             # Module system + all feature modules
│   ├── rbac/               # Role-based access control engine
│   ├── storage/            # Persistence layer (PostgreSQL, SQLite)
│   └── testutil/           # Shared test utilities and fakes
├── pkg/                    # Reusable, domain-agnostic libraries
│   ├── argocd/             # ArgoCD API client
│   ├── health/             # Health check server
│   ├── i18n/               # Internationalization (en, vi)
│   ├── kube/               # Kubernetes client factory
│   ├── logger/             # Structured logging (Zap)
│   ├── redis/              # Redis client, cache, rate limiter
│   ├── telegram/           # Telegram formatting utilities
│   └── version/            # Build version info
├── test/                   # Integration and E2E tests
│   ├── e2e/                # Full end-to-end test suite
│   └── integration/        # Storage + watcher integration tests
├── .agent/                 # Development rules and workflows
├── Makefile                # Build, test, lint tasks
├── docker-compose.local.yaml
├── go.mod / go.sum
└── README.md
```

---

## `cmd/telekube/`

| File | Description |
|------|-------------|
| `main.go` | Application entrypoint. Wires all dependencies: config → storage → redis → cluster manager → RBAC engine → audit logger → module registry → bot. Registers **all** modules into the registry **before** calling `bot.RegisterHandlers()`. Starts the health server, bot polling, and graceful shutdown. |

### Module Registration Order (in `main.go`)

```
1. kubernetes    (if enabled)
2. helm          (if enabled) — needs RESTConfig from cluster manager
3. incident      (if enabled) — needs cluster manager + storage
4. approval      — always registered (callback-only, no slash commands)
5. rbac          — always registered
6. notify        (if enabled)
7. bot.New()     — creates Telegram bot instance (no handlers yet)
8. watcher       (if enabled) — needs bot notifier
9. argocd        (if enabled) — needs ArgoCD instances
10. briefing     (if enabled) — needs bot notifier
11. bot.RegisterHandlers() — registers ALL module handlers with bot
12. health server + app.Run()
```

---

## `configs/`

| File | Description |
|------|-------------|
| `config.local.yaml` | Local development config. Contains: telegram token, database DSN, Redis address, cluster definitions, module toggles, watcher settings, ArgoCD instances. |

---

## `internal/app/`

| File | Description |
|------|-------------|
| `app.go` | `App` struct — orchestrates the application lifecycle: starts health server, bot, module registry. Handles OS signal-based graceful shutdown (SIGINT/SIGTERM). |

---

## `internal/audit/`

| File | Description |
|------|-------------|
| `logger.go` | `Logger` interface for audit trail. Methods: `Log(AuditEntry)`, `List(filters)`. Implementations in `storage/`. |

---

## `internal/bot/`

Core Telegram bot package. Manages bot instance, middleware pipeline, and base command handlers.

| File | Description |
|------|-------------|
| `bot.go` | `Bot` struct. Creates the telebot instance, registers middleware, exposes `RegisterHandlers()` (called from main after all modules are registered), `Start()`, `Stop()`. |
| `notifier.go` | `Notifier` — wraps the bot to send messages to specific chats. Used by watcher and briefing modules for background alerts. |

### `internal/bot/handler/`

| File | Description |
|------|-------------|
| `start.go` | Handlers for `/start`, `/help`, `/clusters`, `/audit`, and `cluster_select` callback. `/help` dynamically groups commands by module with RBAC-based visibility filtering. |
| `start_test.go` | Unit tests for all base handlers (Start, Help permission filtering, Clusters, ClusterSelect). |

### `internal/bot/keyboard/`

| File | Description |
|------|-------------|
| `builder.go` | `Builder` — creates inline keyboards (cluster selector, namespace selector, confirmation dialogs, pod actions, log actions). Uses `CallbackStore` to shorten long callback data. |
| `callback_store.go` | `CallbackStore` — in-memory store that hashes long callback strings (>50 bytes) into short keys to stay under Telegram's 64-byte callback data limit. |

### `internal/bot/middleware/`

| File | Description |
|------|-------------|
| `auth.go` | Authentication middleware. Auto-registers new users, checks admin whitelist, blocks unauthorized users. Stores `entity.User` in context. |
| `audit.go` | Audit middleware. Logs every incoming command/callback to the audit trail. |
| `rate_limit.go` | Per-user rate limiting middleware (in-memory token bucket). |
| `recovery.go` | Panic recovery middleware. Catches panics in handlers and logs them. |

---

## `internal/cluster/`

| File | Description |
|------|-------------|
| `manager.go` | `Manager` interface + `manager` implementation. Manages multiple K8s cluster connections. Methods: `List()`, `Get(name)`, `ClientSet(name)`, `RESTConfig(name)`. Lazy-initializes `kube.Clients` per cluster. |
| `user_context.go` | `UserContext` — tracks which cluster each Telegram user has selected (in-memory map, goroutine-safe). |

---

## `internal/config/`

| File | Description |
|------|-------------|
| `config.go` | Configuration structs loaded via Viper. Sections: `TelegramConfig`, `DatabaseConfig`, `RedisConfig`, `ClusterConfig`, `ModulesConfig` (with per-module `ModuleToggle`), `WatcherConfig`, `ArgoCDConfig`, `ServerConfig`. |

---

## `internal/entity/`

Domain models used across all layers. No external dependencies.

| File | Description |
|------|-------------|
| `user.go` | `User` — Telegram user identity (ID, username, role, active status). |
| `cluster.go` | `ClusterInfo`, `ClusterStatus` — cluster metadata and health status enum. |
| `audit.go` | `AuditEntry` — audit log record (user, action, resource, cluster, status, timestamp). |
| `role.go` | `Role`, `UserRoleBinding` — RBAC role definitions and user-role assignments. Constants: `RoleAdmin`, `RoleOperator`, `RoleViewer`. |
| `approval.go` | `ApprovalRequest`, `Approver` — approval workflow entities. Status enum: Pending, Approved, Rejected, Cancelled, Expired. |
| `notification.go` | `NotificationPreference` — per-user notification settings (severity filter, quiet hours, muted clusters). |
| `health.go` | `HealthStatusHealthy`, `HealthStatusUnhealthy` — module health status enum. |
| `alert.go` | `AlertRule`, `AlertCondition`, `AlertScope`, `AlertNotify` — custom watcher alert rule definitions. |
| `freeze.go` | `DeployFreeze` — ArgoCD deployment freeze entity. |

---

## `internal/leader/`

| File | Description |
|------|-------------|
| `election.go` | Redis-based leader election. Only the leader instance runs background watchers. Uses `SET NX` with TTL + periodic renewal. |

---

## `internal/module/`

The pluggable module system. Every feature is a `Module` that implements:
- `Name()`, `Description()`
- `Register(bot, group)` — register Telegram handlers
- `Start(ctx)`, `Stop(ctx)` — lifecycle
- `Health()` — health status
- `Commands()` — command metadata for `/help`

| File | Description |
|------|-------------|
| `module.go` | `Module` interface and `CommandInfo` struct. |
| `registry.go` | `Registry` — manages module registration, handler registration, lifecycle (StartAll/StopAll), health checks. Key methods: `Register()`, `RegisterAll(bot)`, `ModulesWithCommands()`, `AllCommands()`, `HealthAll()`. |
| `registry_test.go` | Unit tests for registry. |

---

## Feature Modules

### `internal/module/kubernetes/`

The core K8s operations module. **12 commands**.

| File | Description |
|------|-------------|
| `module.go` | Module struct, registration, command definitions, `CallbackStore` integration. |
| `pods.go` | `/pods` — list pods by namespace with status indicators. Interactive pod detail view. |
| `logs.go` | `/logs <pod>` — view container logs with tail options (100/200/500 lines), previous container logs. |
| `events.go` | `/events <pod>` — show Kubernetes events for a pod. |
| `restart.go` | `/restart <pod>` — delete a pod (with approval gate if configured). |
| `top.go` | `/top`, `/top nodes` — resource usage metrics (CPU/memory) for pods and nodes. |
| `scale.go` | `/scale` — scale deployment/statefulset replicas (interactive flow: select deployment → enter count). |
| `nodes.go` | `/nodes` — list cluster nodes with status, CPU, memory, disk. Cordon/uncordon/drain operations. |
| `quota.go` | `/quota` — show namespace resource quotas and LimitRanges. |
| `namespace.go` | Namespace listing helper (shared by multiple handlers). |
| `namespaces.go` | `/namespaces` — list all namespaces. |
| `deploys.go` | `/deploys` — list deployments in a namespace with replica status. |
| `cronjobs.go` | `/cronjobs` — list CronJobs with last schedule and status. |
| `*_test.go` | Unit tests for each handler. |

### `internal/module/helm/`

Helm release management. **1 command**.

| File | Description |
|------|-------------|
| `module.go` | `/helm` — list Helm releases by namespace, view release details, rollback to previous revisions. Integrates `CallbackStore` for safe callback data. Dynamic namespace fetching from cluster. |
| `client.go` | `ReleaseClient` interface + `ClientFactory`. Production factory uses Helm SDK `action.Configuration`. |
| `restconfig.go` | REST config getter for Helm action configuration. |
| `export_test.go` | Test exports for internal types. |
| `module_test.go` | Unit tests with fake Helm client. |

### `internal/module/incident/`

Incident timeline builder. **1 command**.

| File | Description |
|------|-------------|
| `module.go` | `/incident` — builds timeline from K8s events + audit log. Namespace selection → time window (15m/30m/1h/4h) → merged chronological timeline with severity indicators. Integrates `CallbackStore`. |
| `module_test.go` | Unit tests. |

### `internal/module/argocd/`

ArgoCD GitOps operations. **3 commands** (requires ArgoCD instance config).

| File | Description |
|------|-------------|
| `module.go` | Module struct, registration, instance management. |
| `dashboard.go` | `/dashboard` — cross-instance GitOps status overview (sync status, health counts). |
| `sync.go` | `/apps` — list ArgoCD applications with sync/health status. Interactive sync and hard-refresh operations. |
| `rollback.go` | Rollback ArgoCD application to previous revision. |
| `freeze.go` | `/freeze` — deployment freeze management (block sync/rollback during maintenance). |
| `diff.go` | View application diff (live vs desired state). |
| `*_test.go` | Unit/format tests. |

### `internal/module/watcher/`

Real-time Kubernetes monitoring. **0 commands** (background only).

| File | Description |
|------|-------------|
| `module.go` | Module lifecycle, watcher coordination, leader election integration. |
| `pod_watcher.go` | Watches pod status changes: CrashLoopBackOff, OOMKilled, ImagePullBackOff, restarts. |
| `node_watcher.go` | Watches node conditions: NotReady, MemoryPressure, DiskPressure. |
| `cronjob_watcher.go` | Watches CronJob failures and missed schedules. |
| `cert_watcher.go` | Watches TLS certificate expiry (cert-manager Secrets). |
| `pvc_watcher.go` | Watches PersistentVolumeClaim issues (Pending, Lost). |
| `argocd_watcher.go` | Watches ArgoCD application health/sync changes. |
| `custom_rules.go` | User-defined alert rules from config. |
| `*_test.go` | Unit and edge-case tests. |

### `internal/module/notify/`

Notification preferences. **1 command**.

| File | Description |
|------|-------------|
| `module.go` | `/notify` — per-user notification settings: severity filter (info/warning/critical), quiet hours (configurable time windows), cluster muting. |
| `module_test.go` | Unit tests. |

### `internal/module/rbacmod/`

RBAC management UI. **1 command**.

| File | Description |
|------|-------------|
| `module.go` | `/rbac` — admin-only role management: list users, list roles, assign/revoke roles, view user details. Interactive multi-step flow. |
| `module_test.go` | Unit tests. |

### `internal/module/approval/`

Approval workflow for dangerous operations. **0 commands** (callback-only).

| File | Description |
|------|-------------|
| `handler.go` | `BotModule` — handles approval/rejection/cancellation callbacks. Formats approval request and resolution messages. |
| `manager.go` | `Manager` — approval request lifecycle: create, decide, cancel, expiry. Configurable required approvals and timeout. |

### `internal/module/briefing/`

Scheduled cluster briefings. **0 commands** (automatic).

| File | Description |
|------|-------------|
| `module.go` | Module lifecycle, scheduling. |
| `reporter.go` | Generates cluster health summary reports. |
| `scheduler.go` | Cron-based scheduler for periodic briefings. |
| `*_test.go` | Unit and edge-case tests. |

### `internal/module/alertmanager/`

AlertManager webhook receiver. **0 commands** (HTTP webhook).

| File | Description |
|------|-------------|
| `module.go` | HTTP handler for `POST /webhook/alertmanager`. Validates bearer token, parses AlertManager payload, formats and sends alerts to configured Telegram chats. |

---

## `internal/rbac/`

| File | Description |
|------|-------------|
| `engine.go` | `Engine` interface + implementation. RBAC engine with role hierarchy: `admin > operator > viewer`. Methods: `HasPermission()`, `GetRole()`, `SetRole()`, `Authorize()`. |
| `defaults.go` | Default role definitions and permission constants (e.g., `PermKubernetesPodsList`, `PermHelmReleaseslist`, `PermArgoCDAppsList`, `PermAdminRBACManage`). |
| `*_test.go` | Unit tests for engine and defaults. |

---

## `internal/storage/`

Persistence layer with two backends: PostgreSQL (production) and SQLite (development/testing).

| File | Description |
|------|-------------|
| `storage.go` | `Storage` interface. Sub-interfaces: `Users()`, `Audit()`, `Freeze()`, `Roles()`, `RoleBindings()`, `NotificationPrefs()`, `ApprovalRequests()`. |
| `errors.go` | Common storage error types. |

### `internal/storage/postgres/`

| File | Description |
|------|-------------|
| `postgres.go` | PostgreSQL connection pool (`pgxpool`), migration runner, `Storage` implementation. |
| `user_repo.go` | User CRUD operations. |
| `audit_repo.go` | Audit log repository with pagination and filtering. |
| `rbac_repo.go` | Role and role-binding persistence. |
| `freeze_repo.go` | Deployment freeze persistence. |
| `notification_pref_repo.go` | Notification preferences persistence. |

### `internal/storage/postgres/migrations/`

| File | Description |
|------|-------------|
| `000001_initial.up.sql` / `.down.sql` | Users, audit_logs tables. |
| `000002_argocd_freeze.up.sql` / `.down.sql` | Deployment freeze table. |
| `000003_rbac_v2.up.sql` / `.down.sql` | Roles, user_role_bindings tables. |
| `000004_approval.up.sql` / `.down.sql` | Approval requests table. |
| `000005_notification_prefs.up.sql` | Notification preferences table. |

### `internal/storage/sqlite/`

Mirror of PostgreSQL repos using SQLite for local development and testing. Same interface, same table structure.

---

## `internal/testutil/`

| File | Description |
|------|-------------|
| `fake_cluster.go` | `FakeClusterManager`, `FakeClusterManagerError` — test doubles for `cluster.Manager`. |
| `fake_rbac.go` | `FakeRBAC` — test double for `rbac.Engine`. |
| `fake_audit.go` | `FakeAuditLogger` — in-memory audit logger for tests. |
| `fake_telebot.go` | `FakeTelebotContext` — test double for `telebot.Context`. |
| `helpers.go` | Common test helper functions. |

---

## `pkg/` — Shared Libraries

### `pkg/argocd/`
| File | Description |
|------|-------------|
| `client.go` | ArgoCD REST API client: list apps, get app, sync, rollback, diff, hard-refresh. |
| `auth.go` | Token-based authentication for ArgoCD API. |
| `types.go` | ArgoCD domain types (Application, SyncStatus, HealthStatus). |

### `pkg/health/`
| File | Description |
|------|-------------|
| `checker.go` | Health check registry + HTTP server (`/healthz`, `/readyz`). |

### `pkg/i18n/`
| File | Description |
|------|-------------|
| `i18n.go` | Internationalization support. Loads locale YAML files. |
| `locales/en.yaml` | English translations. |
| `locales/vi.yaml` | Vietnamese translations. |

### `pkg/kube/`
| File | Description |
|------|-------------|
| `client.go` | `Clients` struct — wraps `kubernetes.Clientset`, `metrics.Clientset`, and `*rest.Config`. `NewClients(kubeconfig)` creates clients from kubeconfig path or in-cluster config. |

### `pkg/logger/`
| File | Description |
|------|-------------|
| `logger.go` | Zap logger initialization (JSON structured logging, level from env). |
| `fields.go` | Common log field helpers (trace_id, request_id, user_id). |

### `pkg/redis/`
| File | Description |
|------|-------------|
| `client.go` | Redis connection factory with TLS, pool config. |
| `cache.go` | Generic cache-aside helper with TTL and jitter. |
| `rate_limiter.go` | Redis-backed per-user/IP rate limiter (sliding window). |

### `pkg/telegram/`
| File | Description |
|------|-------------|
| `format.go` | Telegram message formatting helpers (truncation, escaping, code blocks). |
| `paginator.go` | Message pagination for long outputs (splits into multiple messages). |
| `progress.go` | Progress bar renderer for long-running operations. |

### `pkg/version/`
| File | Description |
|------|-------------|
| `version.go` | Build-time version information (injected via ldflags). |

---

## `test/`

### `test/e2e/`

Full end-to-end test suite using a fake Telegram server + real K3s cluster.

| File | Description |
|------|-------------|
| `harness.go` | Test harness: sets up fake Telegram server, K8s cluster, storage, bot, all modules. |
| `fake_telegram.go` | In-process fake Telegram Bot API server. |
| `k3s_cluster.go` | K3s cluster lifecycle for E2E tests. |
| `smoke_test.go` | Basic /start, /help, /clusters smoke tests. |
| `pods_e2e_test.go` | /pods command E2E tests. |
| `nodes_e2e_test.go` | /nodes command E2E tests (cordon/uncordon/drain). |
| `scale_e2e_test.go` | /scale command E2E tests. |
| `restart_e2e_test.go` | /restart command E2E tests. |
| `deploys_e2e_test.go` | /deploys command E2E tests. |
| `cronjobs_e2e_test.go` | /cronjobs command E2E tests. |
| `namespaces_e2e_test.go` | /namespaces command E2E tests. |
| `helm_e2e_test.go` | /helm command E2E tests. |
| `rbac_e2e_test.go` | /rbac command E2E tests. |
| `approval_e2e_test.go` | Approval workflow E2E tests. |
| `audit_e2e_test.go` | /audit command E2E tests. |
| `watcher_e2e_test.go` | Watcher alert E2E tests. |

### `test/integration/`
| File | Description |
|------|-------------|
| `storage_test.go` | PostgreSQL/SQLite integration tests. |
| `watcher_test.go` | Watcher integration tests. |

---

## `deploy/`

| Path | Description |
|------|-------------|
| `deploy/docker/Dockerfile` | Multi-stage Docker build (Go builder → Alpine runtime). |
| `deploy/helm/` | Helm chart for Kubernetes deployment (values, templates). |

---

## Configuration

See `configs/config.local.yaml` for full reference. Key sections:

```yaml
telegram:
  token: "BOT_TOKEN"
  admin_ids: [123456789]
  
database:
  driver: postgres    # or sqlite
  dsn: "postgres://..."
  
redis:
  addr: "redis:6379"
  
clusters:
  - name: my-cluster
    kubeconfig: "/path/to/kubeconfig"

modules:
  kubernetes: { enabled: true }
  helm:       { enabled: true }
  incident:   { enabled: true }
  notify:     { enabled: true }
  watcher:    { enabled: true }
  argocd:     { enabled: false }  # needs argocd config
  approval:   { enabled: false }
  briefing:   { enabled: false }  # needs schedule config
```
