# Phần 2 — Cấu Hình

> **Mục tiêu:** Hiểu cấu trúc file cấu hình, các biến môi trường, và tuỳ chỉnh từng module.

---

## 2.1 Thứ tự ưu tiên cấu hình

Telekube áp dụng cấu hình theo thứ tự ưu tiên từ cao xuống thấp:

```
1. Biến môi trường (TELEKUBE_*)   ← cao nhất
2. File configs/config.yaml
3. Giá trị mặc định               ← thấp nhất
```

---

## 2.2 Cấu trúc file cấu hình

Dưới đây là file `configs/config.yaml` đầy đủ có chú thích:

```yaml
# ─────────────────────────────────────────────
# TELEGRAM
# ─────────────────────────────────────────────
telegram:
  # Bot token từ @BotFather (bắt buộc)
  token: "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"

  # Danh sách Telegram User ID có quyền admin (bắt buộc, ít nhất 1)
  admin_ids: [123456789]

  # Webhook URL (tuỳ chọn). Để trống → dùng long polling.
  # webhook_url: "https://your-domain.com/webhook"

  # Danh sách chat ID được phép dùng bot (tuỳ chọn).
  # Để trống → cho phép tất cả.
  # Các chat này cũng nhận cảnh báo từ watcher và briefing.
  # allowed_chats: [-1001234567890]

  # Giới hạn tốc độ: số tin nhắn tối đa mỗi phút mỗi người
  rate_limit: 30

# ─────────────────────────────────────────────
# KUBERNETES CLUSTERS
# ─────────────────────────────────────────────
clusters:
  - name: "production"            # Tên định danh (không dấu cách)
    display_name: "Production"    # Tên hiển thị trong Telegram
    kubeconfig: ""                # Đường dẫn kubeconfig (để trống = ~/.kube/config)
    context: ""                   # Context name (để trống = current-context)
    in_cluster: false             # true nếu chạy trong cluster
    default: true

  # Có thể thêm nhiều cluster
  - name: "staging"
    display_name: "Staging"
    kubeconfig: "/home/user/.kube/staging-config"
    context: "staging-context"
    default: false

# ─────────────────────────────────────────────
# ARGOCD
# ─────────────────────────────────────────────
argocd:
  insecure: false        # true chỉ cho development/test
  timeout: 30s           # Timeout cho ArgoCD API
  instances:
    - name: "prod-argocd"
      url: "https://argocd.example.com"
      auth:
        type: "token"    # hoặc "oauth"
        token: "${ARGOCD_TOKEN}"
      # Liên kết với tên cluster ở trên
      clusters:
        - "production"

# ─────────────────────────────────────────────
# STORAGE
# ─────────────────────────────────────────────
storage:
  # sqlite (development) hoặc postgres (production)
  backend: sqlite

  sqlite:
    path: "telekube.db"

  # PostgreSQL (khuyến nghị cho production)
  # postgres:
  #   dsn: "postgres://user:pass@localhost:5432/telekube?sslmode=disable"
  #   max_open_conns: 50
  #   max_idle_conns: 25
  #   conn_max_lifetime_min: 30

  # Redis (tuỳ chọn — bật caching và rate limiting nâng cao)
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
    enabled: true      # Bật module K8s (mặc định)

  argocd:
    enabled: false     # Bật nếu có ArgoCD instance

  watcher:
    enabled: false     # Bật real-time alerts

  approval:
    enabled: false     # Bật approval workflow

  briefing:
    enabled: false     # Bật báo cáo hằng ngày
    schedule: "0 8 * * *"           # Cron: 8 giờ sáng hằng ngày
    timezone: "Asia/Ho_Chi_Minh"

  alertmanager:
    enabled: false     # Bật Prometheus AlertManager webhook

  helm:
    enabled: false     # Bật Helm management

  incident:
    enabled: false     # Bật Incident timeline

  notify:
    enabled: false     # Bật notification preferences (/notify)

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
  # Vai trò mặc định cho người dùng mới
  default_role: viewer   # viewer | operator | admin | on-call | super-admin

# ─────────────────────────────────────────────
# LEADER ELECTION (HA mode — nhiều replica)
# ─────────────────────────────────────────────
# leader_election:
#   enabled: false
#   namespace: "telekube"
```

---

## 2.3 Biến môi trường

Mọi cấu hình đều có thể được override bằng biến môi trường với prefix `TELEKUBE_`.

Quy tắc đặt tên: thay dấu `.` thành `_` và viết HOA toàn bộ.

| Biến môi trường | Tương đương trong YAML | Ví dụ |
|----------------|------------------------|-------|
| `TELEKUBE_TELEGRAM_TOKEN` | `telegram.token` | `bot123:ABC...` |
| `TELEKUBE_TELEGRAM_ADMIN_IDS` | `telegram.admin_ids` | `123456789,987654321` |
| `TELEKUBE_TELEGRAM_RATE_LIMIT` | `telegram.rate_limit` | `30` |
| `TELEKUBE_STORAGE_BACKEND` | `storage.backend` | `sqlite` hoặc `postgres` |
| `TELEKUBE_STORAGE_SQLITE_PATH` | `storage.sqlite.path` | `telekube.db` |
| `TELEKUBE_STORAGE_POSTGRES_DSN` | `storage.postgres.dsn` | `postgres://...` |
| `TELEKUBE_LOG_LEVEL` | `log.level` | `debug` |
| `TELEKUBE_LOG_FORMAT` | `log.format` | `json` |
| `TELEKUBE_SERVER_PORT` | `server.port` | `8080` |
| `TELEKUBE_RBAC_DEFAULT_ROLE` | `rbac.default_role` | `viewer` |
| `TELEKUBE_MODULES_ARGOCD_ENABLED` | `modules.argocd.enabled` | `true` |
| `TELEKUBE_MODULES_WATCHER_ENABLED` | `modules.watcher.enabled` | `true` |

### Ví dụ: Cấu hình chỉ bằng biến môi trường

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

## 2.4 Cấu hình Multi-Cluster

Telekube hỗ trợ quản lý nhiều cluster đồng thời:

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

Người dùng chuyển đổi cluster bằng lệnh `/clusters` trong Telegram.

---

## 2.5 Cấu hình ArgoCD

Nếu có nhiều instance ArgoCD, mỗi instance có thể được liên kết với 1 hoặc nhiều cluster:

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

> **Bảo mật:** Lưu token ArgoCD trong biến môi trường hoặc secret manager, không đưa vào file cấu hình commit lên git.

---

## 2.6 Cấu hình Briefing (Báo cáo hằng ngày)

```yaml
modules:
  briefing:
    enabled: true
    # Cron expression: phút giờ ngày tháng thứ
    schedule: "0 8 * * *"        # 8:00 sáng mỗi ngày
    timezone: "Asia/Ho_Chi_Minh" # Múi giờ Việt Nam
```

Các múi giờ phổ biến:
| Múi giờ | IANA Name |
|---------|-----------|
| Việt Nam (UTC+7) | `Asia/Ho_Chi_Minh` |
| Thái Lan (UTC+7) | `Asia/Bangkok` |
| Singapore (UTC+8) | `Asia/Singapore` |
| UTC | `UTC` |

---

## 2.7 Cấu hình HA (High Availability)

Khi chạy nhiều replica, chỉ 1 pod thực hiện watcher và briefing (leader election qua Kubernetes Lease):

```yaml
leader_election:
  enabled: true
  namespace: "telekube"   # Namespace để tạo Lease resource
```

Khi bật leader election, cần thêm RBAC cho ServiceAccount:

```yaml
# Helm values
leaderElection:
  enabled: true
```

---

## Bước tiếp theo

- [Hướng dẫn Kubernetes →](03-kubernetes.md)
- [Hướng dẫn ArgoCD →](04-argocd.md)
