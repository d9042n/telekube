# Phần 6 — Giám Sát & Cảnh Báo (Watcher)

> **Mục tiêu:** Nhận cảnh báo real-time lên Telegram khi có sự cố trên cluster: pod crash, OOMKill, node down, cert sắp hết hạn, PVC đầy, CronJob lỗi.

---

## 6.1 Yêu cầu & Kích hoạt

```yaml
modules:
  watcher:
    enabled: true

telegram:
  # Chat nào nhận cảnh báo (ngoài admin)
  allowed_chats: [-1001234567890]  # ID của group chat nhóm on-call
```

> **Quan trọng:** Watcher chỉ gửi cảnh báo vào các chat được liệt kê trong `allowed_chats`. Admin chat luôn nhận được.

---

## 6.2 Các loại Watcher

### 6.2.1 Pod Watcher

**Theo dõi:** Trạng thái pod thay đổi bất thường

**Cảnh báo khi:**
- Pod vào trạng thái `CrashLoopBackOff`
- Pod bị `OOMKilled` (hết memory)
- Pod không thể start (`ImagePullBackOff`, `ErrImagePull`)
- Pod `Pending` quá lâu (> 5 phút)

**Ví dụ cảnh báo:**

```
🔴 Pod Crash Alert

Cluster: production
Namespace: backend
Pod: api-server-7d8f9b-xvz4k

Status: CrashLoopBackOff
Restarts: 12
Reason: OOMKilled — dùng 512Mi, limit 256Mi

[Logs] [Events] [Mute 1h]
```

**Nút hành động:**
- **Logs** — xem log pod trực tiếp
- **Events** — xem K8s events
- **Mute 1h** — tắt cảnh báo cho pod này trong 1 giờ

---

### 6.2.2 Node Watcher

**Theo dõi:** Trạng thái node trong cluster

**Cảnh báo khi:**
- Node chuyển sang `NotReady`
- Node bị `MemoryPressure`, `DiskPressure`, hoặc `PIDPressure`
- Node mất kết nối (`Unknown`)

**Ví dụ cảnh báo:**

```
⚡ Node Alert

Cluster: production
Node: worker-node-3

Status: NotReady
Condition: MemoryPressure
Duration: 2m 30s

[Node Details] [Mute 1h]
```

---

### 6.2.3 CronJob Watcher

**Theo dõi:** Kết quả thực hiện CronJob

**Cảnh báo khi:**
- CronJob job thất bại (exit code ≠ 0)
- CronJob miss scheduled run (bị trễ)
- Số lần thất bại liên tiếp vượt ngưỡng

**Ví dụ cảnh báo:**

```
⏰ CronJob Failed

Cluster: production
Namespace: batch
CronJob: nightly-report

Job: nightly-report-28501200
Status: Failed
Started: 02:00:00 UTC
Duration: 4m 23s

[Job Logs] [Mute 1h]
```

> **Namespace loại trừ:** `kube-system` bị loại trừ mặc định để tránh alert noise từ system cronjob.

---

### 6.2.4 Certificate Watcher

**Theo dõi:** Chứng chỉ TLS lưu trong Kubernetes Secrets (loại `kubernetes.io/tls`)

**Cảnh báo khi:**
- Cert hết hạn trong **30 ngày**
- Cert hết hạn trong **7 ngày** (cảnh báo khẩn)
- Cert đã hết hạn

**Ví dụ cảnh báo:**

```
🔐 Certificate Expiry Warning

Cluster: production
Namespace: ingress-nginx
Secret: api-tls-cert

Common Name: api.example.com
Expires: 2026-04-01 (14 ngày nữa)
Issuer: Let's Encrypt Authority X3

[Mute 7d]
```

> **Namespace loại trừ:** `kube-system` bị loại trừ mặc định.

---

### 6.2.5 PVC Watcher

**Theo dõi:** Dung lượng PersistentVolumeClaim

**Cảnh báo khi:**
- PVC dùng > **85%** dung lượng
- PVC dùng > **95%** dung lượng (cảnh báo khẩn)
- PVC ở trạng thái `Pending` (chưa được bind)

**Ví dụ cảnh báo:**

```
💾 PVC Usage Alert

Cluster: production
Namespace: database
PVC: postgres-data-pvc

Used: 85.2% (8.5Gi / 10Gi)
Available: 1.5Gi

[Mute 1h]
```

---

### 6.2.6 ArgoCD Watcher

**Theo dõi:** Trạng thái ứng dụng ArgoCD theo thời gian thực

**Cảnh báo khi:**
- App chuyển sang `Degraded`
- App bị `OutOfSync` lâu (không tự sync được)
- Sync thất bại

**Ví dụ cảnh báo:**

```
🚀 ArgoCD App Alert

Instance: prod-argocd
App: backend-api

Health: Degraded
Sync: OutOfSync
Message: Deployment "backend-api" has unavailable replicas

[View App] [Sync] [Mute 1h]
```

---

## 6.3 Custom Alert Rules

Ngoài các watcher tích hợp, bạn có thể định nghĩa **custom alert rules** trong cấu hình, được đánh giá định kỳ (mỗi 30 giây).

### Các loại condition hỗ trợ

| Loại | Mô tả |
|------|-------|
| `pod_restart_count` | Cảnh báo khi container restart vượt ngưỡng |
| `pod_pending_duration` | Cảnh báo khi pod Pending quá lâu |
| `namespace_quota_percentage` | Cảnh báo khi quota vượt phần trăm |
| `deployment_unavailable` | Cảnh báo khi deployment không có replica available |

### Ví dụ cấu hình

```yaml
watcher:
  custom_rules:
    - name: "high-restarts"
      description: "Container restart count quá cao"
      severity: "warning"
      condition:
        type: "pod_restart_count"
        threshold: 5
      scope:
        clusters: ["*"]
        namespaces: ["production"]

    - name: "deployment-down"
      description: "Deployment không có replica available"
      severity: "critical"
      condition:
        type: "deployment_unavailable"
      scope:
        clusters: ["production"]
        namespaces: ["*"]
```

### Tính năng

- **Phạm vi đánh giá**: Giới hạn rules cho clusters/namespaces cụ thể
- **Deduplication**: Cooldown 10 phút tránh cảnh báo lặp
- **Audit logging**: Mọi custom alert đều được ghi audit log
- **Thông báo riêng**: Route alerts đến chats cụ thể

---

## 6.4 Anti-Spam & Deduplication

Watcher tự động tránh spam cảnh báo lặp lại:

- **Cooldown:** Mỗi alert có cooldown 5 phút — nếu cùng một sự cố vẫn còn, không gửi lại
- **Deduplication:** Các alert giống nhau (cùng pod/node) được gộp lại
- **Mute:** Người dùng có thể nhấn **Mute 1h** để tắt alert cho resource cụ thể trong 1 giờ

---

## 6.5 AlertManager Webhook

Ngoài các watcher tích hợp, Telekube hỗ trợ nhận cảnh báo từ **Prometheus AlertManager** qua webhook:

### Kích hoạt

```yaml
modules:
  alertmanager:
    enabled: true

server:
  port: 8080
```

### Cấu hình AlertManager

Thêm webhook receiver vào file cấu hình Prometheus AlertManager:

```yaml
# alertmanager.yml
receivers:
  - name: 'telekube'
    webhook_configs:
      - url: 'http://telekube:8080/api/v1/alertmanager/webhook'
        send_resolved: true

route:
  receiver: 'telekube'
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
```

### Ví dụ cảnh báo từ AlertManager

```
🚨 Alert: HighMemoryUsage

Cluster: production
Severity: critical

Labels:
  alertname: HighMemoryUsage
  namespace: backend
  pod: api-server-7d8f9b

Annotations:
  summary: Pod memory usage > 90%
  description: Pod api-server-7d8f9b đang dùng 92% memory limit

Status: firing
Started: 2026-03-18 14:00 UTC
```

---

## 6.6 Cấu hình nâng cao Watcher

Các watcher được cấu hình tự động khi bật module. Nếu cần tuỳ chỉnh exclusion list:

```go
// Ví dụ: loại trừ thêm namespace
m.certWatcher = NewCertWatcher(clusters, notifier, audit, cfg, CertWatcherConfig{
    ExcludeNamespaces: []string{"kube-system", "monitoring", "logging"},
}, logger)
```

> Hiện tại việc tuỳ chỉnh exclusion list cần sửa code. Sẽ hỗ trợ cấu hình trong file config ở phiên bản sau.

---

## 6.7 Bảng tóm tắt Watcher

| Watcher | Theo dõi | Ngưỡng cảnh báo |
|---------|---------|-----------------|
| Pod Watcher | Trạng thái pod | CrashLoopBackOff, OOMKill, Pending > 5min |
| Node Watcher | Trạng thái node | NotReady, MemoryPressure, DiskPressure |
| CronJob Watcher | Kết quả CronJob | Job Failed, Miss Schedule |
| Cert Watcher | Hết hạn TLS cert | < 30 ngày, < 7 ngày |
| PVC Watcher | Dung lượng PVC | > 85%, > 95% |
| ArgoCD Watcher | Trạng thái ArgoCD app | Degraded, OutOfSync kéo dài |

---

## Bước tiếp theo

- [Incident & Briefing →](07-incident-briefing.md)
- [RBAC & Phân quyền →](08-rbac.md)
