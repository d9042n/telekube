# Part 2 — Configuration

> **Objective:** Understand the configuration file structure, environment variables, and customize each module.

---

## 2.1 Configuration Precedence

Telekube applies configuration in the following order of precedence, from highest to lowest:

```
1. Environment variables (TELEKUBE_*)   ← highest
2. File configs/config.yaml
3. Default values               ← lowest
```

---

## 2.2 Configuration File Structure

Below is the fully annotated `configs/config.yaml` file:

```yaml
# ─────────────────────────────────────────────
# TELEGRAM
# ─────────────────────────────────────────────
telegram:
  # Bot token from @BotFather (required)
  token: "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"

  # List of Telegram User IDs with admin rights (required, at least 1)
  admin_ids: [123456789]

  # Webhook URL (optional). Leave empty → use long polling.
  # webhook_url: "https://your-domain.com/webhook"

  # List of allowed chat IDs to use the bot (optional).
  # Leave empty → allow all.
  # These chats also receive alerts from watcher and briefing.
  # allowed_chats: [-1001234567890]

  # Rate limit: max messages per minute per user
  rate_limit: 30

# ─────────────────────────────────────────────
# KUBERNETES CLUSTERS
# ─────────────────────────────────────────────
clusters:
  - name: "production"            # Identifier name (no spaces)
    display_name: "Production"    # Display name in Telegram
    kubeconfig: ""                # Kubeconfig path (leave empty = ~/.kube/config)
    context: ""                   # Context name (leave empty = current-context)
    in_cluster: false             # true if running inside the cluster
    default: true

  # You can add multiple clusters
  - name: "staging"
    display_name: "Staging"
    kubeconfig: "/home/user/.kube/staging-config"
    context: "staging-context"
    default: false

# ─────────────────────────────────────────────
# ARGOCD
# ─────────────────────────────────────────────
argocd:
  insecure: false        # true only for development/test
  timeout: 30s           # Timeout for ArgoCD API
  instances:
    - name: "prod-argocd"
      url: "https://argocd.example.com"
      auth:
        type: "token"    # or "oauth"
        token: "${ARGOCD_TOKEN}"
      # Map to cluster names defined above
      clusters:
        - "production"

# ─────────────────────────────────────────────
# STORAGE
# ─────────────────────────────────────────────
storage:
  # sqlite (development) or postgres (production)
  backend: sqlite

  sqlite:
    path: "telekube.db"

  # PostgreSQL (recommended for production)
  # postgres:
  #   dsn: "postgres://user:pass@localhost:5432/telekube?sslmode=disable"
  #   max_open_conns: 50
  #   max_idle_conns: 25
  #   conn_max_lifetime_min: 30

  # Redis (optional — enables caching and advanced rate limiting)
  # redis:
  #   addr: "localhost:6379"
  #   password: ""
  #   db: 0
  #   tls_enable: false
  #   pool_size: 20
  #   op_timeout_ms: 2000

# ─────────────────────────────────────────────
# MODULES
# ─────────────────────────────────────────────
modules:
  kubernetes:
    enabled: true      # Enable K8s module (default)

  argocd:
    enabled: false     # Enable if you have an ArgoCD instance

  watcher:
    enabled: false     # Enable real-time alerts

  approval:
    enabled: false     # Enable approval workflow

  briefing:
    enabled: false     # Enable daily reports
    schedule: "0 8 * * *"           # Cron: 8 AM daily
    timezone: "Asia/Ho_Chi_Minh"

  alertmanager:
    enabled: false     # Enable Prometheus AlertManager webhook

  helm:
    enabled: false     # Enable Helm management

  incident:
    enabled: false     # Enable Incident timeline

  notify:
    enabled: false     # Enable notification preferences (/notify)

# ─────────────────────────────────────────────
# HTTP SERVER (health/metrics)
# ─────────────────────────────────────────────
server:
  port: 8080
  read_timeout_ms: 15000
  write_timeout_ms: 15000
  idle_timeout_ms: 60000

# ─────────────────────────────────────────────
# LOGGING
# ─────────────────────────────────────────────
log:
  level: info       # debug | info | warn | error
  format: console   # json (production) | console (development)

# ─────────────────────────────────────────────
# RBAC
# ─────────────────────────────────────────────
rbac:
  # Default role for new users
  default_role: viewer   # viewer | operator | admin | on-call | super-admin

# ─────────────────────────────────────────────
# LEADER ELECTION (HA mode — multiple replicas)
# ─────────────────────────────────────────────
# leader_election:
#   enabled: false
#   namespace: "telekube"
```

---

## 2.3 Environment Variables

All configurations can be overridden using environment variables prefixed with `TELEKUBE_`.

Naming convention: replace `.` with `_` and UPPERCASE everything.

| Environment Variable | YAML Equivalent | Example |
|----------------------|-----------------|---------|
| `TELEKUBE_TELEGRAM_TOKEN` | `telegram.token` | `bot123:ABC...` |
| `TELEKUBE_TELEGRAM_ADMIN_IDS` | `telegram.admin_ids` | `123456789,987654321` |
| `TELEKUBE_TELEGRAM_RATE_LIMIT` | `telegram.rate_limit` | `30` |
| `TELEKUBE_STORAGE_BACKEND` | `storage.backend` | `sqlite` or `postgres` |
| `TELEKUBE_STORAGE_SQLITE_PATH` | `storage.sqlite.path` | `telekube.db` |
| `TELEKUBE_STORAGE_POSTGRES_DSN` | `storage.postgres.dsn` | `postgres://...` |
| `TELEKUBE_LOG_LEVEL` | `log.level` | `debug` |
| `TELEKUBE_LOG_FORMAT` | `log.format` | `json` |
| `TELEKUBE_SERVER_PORT` | `server.port` | `8080` |
| `TELEKUBE_RBAC_DEFAULT_ROLE` | `rbac.default_role` | `viewer` |
| `TELEKUBE_MODULES_ARGOCD_ENABLED` | `modules.argocd.enabled` | `true` |
| `TELEKUBE_MODULES_WATCHER_ENABLED` | `modules.watcher.enabled` | `true` |

### Example: Configuration strictly via environment variables

```bash
export TELEKUBE_TELEGRAM_TOKEN="123456789:ABC..."
export TELEKUBE_TELEGRAM_ADMIN_IDS="123456789"
export TELEKUBE_STORAGE_BACKEND="postgres"
export TELEKUBE_STORAGE_POSTGRES_DSN="postgres://user:pass@db:5432/telekube"
export TELEKUBE_LOG_LEVEL="info"
export TELEKUBE_LOG_FORMAT="json"
export TELEKUBE_MODULES_KUBERNETES_ENABLED="true"
export TELEKUBE_MODULES_WATCHER_ENABLED="true"
export TELEKUBE_MODULES_ARGOCD_ENABLED="true"

./bin/telekube serve
```

---

## 2.4 Multi-Cluster Configuration

Telekube supports managing multiple clusters simultaneously:

```yaml
clusters:
  - name: "prod-us-east"
    display_name: "Production US-East"
    kubeconfig: "/home/ops/.kube/prod-us-east.yaml"
    default: true

  - name: "prod-eu-west"
    display_name: "Production EU-West"
    kubeconfig: "/home/ops/.kube/prod-eu-west.yaml"
    default: false

  - name: "staging"
    display_name: "Staging"
    kubeconfig: "/home/ops/.kube/staging.yaml"
    default: false
```

Users can switch clusters using the `/clusters` command in Telegram.

---

## 2.5 ArgoCD Configuration

If there are multiple ArgoCD instances, each instance can be linked to 1 or more clusters:

```yaml
argocd:
  insecure: false
  timeout: 30s
  instances:
    - name: "prod-argocd"
      url: "https://argocd.prod.example.com"
      auth:
        type: "token"
        token: "${ARGOCD_PROD_TOKEN}"
      clusters:
        - "prod-us-east"
        - "prod-eu-west"

    - name: "staging-argocd"
      url: "https://argocd.staging.example.com"
      auth:
        type: "token"
        token: "${ARGOCD_STAGING_TOKEN}"
      clusters:
        - "staging"
```

> **Security:** Store the ArgoCD token locally in environment variables or a secret manager. **Do not** put it in configuration files committed to git.

---

## 2.6 Briefing Configuration (Daily Reports)

```yaml
modules:
  briefing:
    enabled: true
    # Cron expression: minute hour day month weekday
    schedule: "0 8 * * *"        # 8:00 AM every day
    timezone: "Asia/Ho_Chi_Minh" # Vietnam Time
```

Common timezones:
| Timezone | IANA Name |
|----------|-----------|
| Vietnam (UTC+7) | `Asia/Ho_Chi_Minh` |
| Thailand (UTC+7) | `Asia/Bangkok` |
| Singapore (UTC+8) | `Asia/Singapore` |
| UTC | `UTC` |

---

## 2.7 HA (High Availability) Configuration

When running multiple replicas, only 1 pod performs watcher and briefing tasks (leader election via Kubernetes Lease):

```yaml
leader_election:
  enabled: true
  namespace: "telekube"   # Namespace to create the Lease resource
```

When leader election is enabled, add RBAC permissions for the ServiceAccount:

```yaml
# Helm values
leaderElection:
  enabled: true
```

---

## Next Steps

- [Kubernetes Guide →](03-kubernetes.md)
- [ArgoCD Guide →](04-argocd.md)
