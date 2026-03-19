# 🚀 Telekube — Tổng Hợp Tính Năng

> Telegram Bot quản lý Kubernetes cluster, ArgoCD, Helm và giám sát real-time — tất cả ngay trong Telegram.

---

## 📋 Mục Lục

1. [Kiến Trúc Tổng Quan](#kiến-trúc-tổng-quan)
2. [Module Kubernetes](#module-kubernetes)
3. [Module ArgoCD](#module-argocd)
4. [Module Helm](#module-helm)
5. [Module Watcher (Giám Sát Real-Time)](#module-watcher)
6. [Module Briefing (Báo Cáo Hàng Ngày)](#module-briefing)
7. [Module Approval (Quy Trình Phê Duyệt)](#module-approval)
8. [Module Incident (Timeline Sự Cố)](#module-incident)
9. [Module AlertManager](#module-alertmanager)
10. [Module Quản Lý RBAC](#module-quản-lý-rbac)
11. [Module Notification Preferences](#module-notification-preferences)
12. [Hệ Thống RBAC](#hệ-thống-rbac)
13. [Hệ Thống Audit](#hệ-thống-audit)
14. [Leader Election](#leader-election)
15. [Multi-Cluster](#multi-cluster)
16. [Bảo Mật](#bảo-mật)
17. [Observability](#observability)

---

## Kiến Trúc Tổng Quan

Telekube được xây dựng theo kiến trúc **modular plugin** — mỗi tính năng là một module độc lập:

```
┌─────────────────────────────────────────────────┐
│                  Telegram Bot                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
│  │Middleware │ │ Handlers │ │  Keyboard Builder│ │
│  │(Auth,Rate,│ │(/start,  │ │  (Inline Buttons)│ │
│  │ Audit)   │ │ /help)   │ │                  │ │
│  └──────────┘ └──────────┘ └──────────────────┘ │
├─────────────────────────────────────────────────┤
│              Module Registry                     │
│  ┌──────┐ ┌──────┐ ┌────┐ ┌───────┐ ┌────────┐ │
│  │ K8s  │ │ArgoCD│ │Helm│ │Watcher│ │Briefing│ │
│  └──────┘ └──────┘ └────┘ └───────┘ └────────┘ │
│  ┌────────┐ ┌────────┐ ┌────────────┐           │
│  │Approval│ │Incident│ │AlertManager│           │
│  └────────┘ └────────┘ └────────────┘           │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌────┐ ┌───────┐ ┌────────────────┐  │
│  │ RBAC │ │Audit│ │Cluster│ │Leader Election │  │
│  │Engine│ │Logger││Manager│ │   (K8s Lease)  │  │
│  └──────┘ └────┘ └───────┘ └────────────────┘  │
├─────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────────┐ ┌───────┐               │
│  │SQLite│ │PostgreSQL │ │ Redis │               │
│  └──────┘ └──────────┘ └───────┘               │
└─────────────────────────────────────────────────┘
```

---

## Module Kubernetes

Module cốt lõi cho phép quản lý Kubernetes cluster trực tiếp từ Telegram.

### Lệnh Có Sẵn

| Lệnh | Mô Tả | Quyền |
|-------|--------|-------|
| `/pods` | Liệt kê pods theo namespace | `kubernetes.pods.list` |
| `/pods <namespace>` | Xem pods trong namespace cụ thể | `kubernetes.pods.list` |
| `/logs <pod>` | Xem log của pod | `kubernetes.pods.logs` |
| `/events <pod>` | Xem events của pod | `kubernetes.pods.events` |
| `/top` | Hiển thị CPU/RAM của pods | `kubernetes.metrics.view` |
| `/top nodes` | Hiển thị CPU/RAM của nodes | `kubernetes.metrics.view` |
| `/scale` | Scale deployment/statefulset | `kubernetes.deployments.scale` |
| `/restart <pod>` | Restart pod (xoá + tạo lại) | `kubernetes.pods.restart` |
| `/nodes` | Liệt kê và quản lý nodes | `kubernetes.nodes.view` |
| `/quota` | Xem resource quotas | `kubernetes.quota.view` |
| `/namespaces` | Liệt kê tất cả namespaces | `kubernetes.namespaces.list` |
| `/deploys [ns]` | Liệt kê deployments | `kubernetes.deployments.list` |
| `/cronjobs [ns]` | Trạng thái CronJobs | `kubernetes.cronjobs.list` |

### Tính Năng Chi Tiết

#### 📦 Quản Lý Pods
- **Liệt kê pods** với status emoji (✅ Running, 🟡 Pending, 🔴 Failed, ⚪ Unknown)
- **Phân trang** khi có nhiều pods, bấm nút ◀️ ▶️ để chuyển trang
- **Chi tiết pod**: IP, node, uptime, restart count, container statuses
- **Xem logs**: Chọn container, cuộn logs (📄 More / ⬆️ Previous)
- **Xem events**: Kubernetes events liên quan đến pod
- **Restart pod**: Xóa pod để controller tạo lại (có xác nhận ✅/❌)

#### 📊 Resource Metrics
- **Top pods**: CPU/RAM sử dụng, sắp xếp theo mức tiêu thụ
- **Top nodes**: Tổng hợp CPU/RAM, allocatable vs used
- **Progress bar** trực quan cho phần trăm sử dụng

#### ⚖️ Scale Operations
- **Chọn namespace → deployment/statefulset → replicas mới**
- **Xác nhận trước khi scale** (chống nhầm)
- **Hỗ trợ scale to 0** (offline workload)

#### 🖥️ Node Management
- **Chi tiết node**: CPU, memory, OS, kubelet version, conditions
- **Cordon/Uncordon**: Đánh dấu node không nhận pod mới
- **Drain**: Di chuyển pods sang node khác (có grace period)
- **Top pods on node**: Xem pods đang chạy trên node cụ thể

---

## Module ArgoCD

Tích hợp ArgoCD GitOps — quản lý applications, sync, rollback và deployment freeze.

### Lệnh Có Sẵn

| Lệnh | Mô Tả | Quyền |
|-------|--------|-------|
| `/apps` | Liệt kê ArgoCD applications | `argocd.apps.list` |
| `/dashboard` | Dashboard tổng hợp GitOps | `argocd.apps.list` |
| `/freeze` | Quản lý deployment freeze | `argocd.freeze.manage` |

### Tính Năng Chi Tiết

#### 📱 Application Management
- **Liệt kê apps** với sync/health status (✅ Synced, 🟡 OutOfSync, 🔴 Degraded)
- **Chi tiết app**: Git revision, source repo, target revision, resources
- **Hỗ trợ nhiều ArgoCD instances** — chuyển đổi dễ dàng

#### 🔄 Sync Operations
- **Sync Now**: Đồng bộ app với Git
- **Sync with Prune**: Xóa resources không còn trong Git
- **Force Sync**: Bỏ qua đang sync, sync ngay
- **Xác nhận trước khi thực hiện**

#### ⏪ Rollback
- **Chọn revision cũ** để rollback
- **Hiển thị lịch sử revision** với thời gian và status

#### 🧊 Deployment Freeze
- **Tạo freeze window**: Chặn sync/rollback trong khoảng thời gian
- **Chọn scope**: Tất cả apps hoặc apps cụ thể
- **Chọn thời lượng**: 30 phút, 1h, 2h, 4h, 8h, 24h
- **Thaw**: Hủy freeze sớm
- **Lịch sử freeze**: Xem các lần freeze trước

#### 📊 GitOps Dashboard
- Tổng hợp trạng thái tất cả apps trên tất cả instances
- **Quick filter**: Xem apps OutOfSync hoặc Degraded
- **Refresh** để cập nhật trạng thái

---

## Module Helm

Quản lý Helm releases — liệt kê, xem chi tiết và rollback.

### Lệnh Có Sẵn

| Lệnh | Mô Tả | Quyền |
|-------|--------|-------|
| `/helm` | Liệt kê Helm releases | `helm.releases.list` |

### Tính Năng Chi Tiết

- **Chọn namespace** → hiển thị releases với status emoji
- **Chi tiết release**: Chart version, app version, revision, last deployed
- **Lịch sử revision**: Xem tất cả revisions
- **Rollback**: Chọn revision cũ để rollback (cần quyền `helm.releases.rollback`)
- **Refresh**: Cập nhật danh sách

---

## Module Watcher

Giám sát Kubernetes real-time — gửi cảnh báo qua Telegram khi phát hiện vấn đề.

### Các Watcher

| Watcher | Giám Sát | Cảnh Báo Khi |
|---------|----------|-------------|
| **PodWatcher** | Pods trên tất cả clusters | OOMKilled, CrashLoopBackOff, ImagePullBackOff, PendingTooLong, Evicted, HighRestarts |
| **NodeWatcher** | Nodes trên tất cả clusters | NotReady, Unreachable, DiskPressure, MemoryPressure, PIDPressure |
| **CronJobWatcher** | CronJobs | Job failed, deadline exceeded |
| **CertWatcher** | Certificates (cert-manager) | Certificate sắp hết hạn, renewal failed |
| **PVCWatcher** | PersistentVolumeClaims | Disk usage cao (Warning ≥80%, Critical ≥90%) |
| **ArgoCDWatcher** | ArgoCD apps | App bị OutOfSync hoặc Degraded |

### Tính Năng Chi Tiết

#### 🚨 Alert System
- **Alert deduplication**: Không gửi cảnh báo trùng lặp trong cooldown (5 phút mặc định)
- **Alert severity**: 🔴 Critical, ⚠️ Warning, ℹ️ Info
- **Mute button**: Bấm 🔇 để tạm dừng cảnh báo 1 giờ
- **Inline buttons**: Từ cảnh báo → xem top pods, xem details

#### 📊 PVC Usage
- **Progress bar trực quan**: `[████████░░] 85%`
- **Hiển thị dung lượng**: `8.5GiB / 10.0GiB`
- **Hỗ trợ annotations**: Đọc `telekube.io/pvc-usage-bytes`
- **Loại trừ namespace**: Cấu hình `exclude_namespaces`

#### ⚡ Chỉ chạy trên Leader
- Watcher chỉ chạy trên replica được bầu làm leader (tránh cảnh báo trùng)
- Tự động dừng khi mất quyền leader

#### 📜 Custom Alert Rules
- **Rules theo cấu hình**: Định nghĩa rules cảnh báo tùy chỉnh trong YAML config
- **Các loại condition hỗ trợ**:
  - `pod_restart_count` — Cảnh báo khi container restart vượt threshold
  - `pod_pending_duration` — Cảnh báo khi pod Pending quá lâu
  - `namespace_quota_percentage` — Cảnh báo khi resource quota vượt phần trăm
  - `deployment_unavailable` — Cảnh báo khi deployment không có replica nào available
- **Phạm vi đánh giá**: Giới hạn rules cho clusters/namespaces cụ thể
- **Deduplication 10 phút**: Tránh cảnh báo lặp lại
- **Thông báo riêng**: Route alerts đến chats cụ thể
- **Audit đầy đủ**: Tất cả custom alerts đều được ghi audit log

---

## Module Briefing

Báo cáo sức khỏe cluster hàng ngày — gửi tự động vào giờ đã cấu hình.

### Cấu Hình

```yaml
modules:
  briefing:
    enabled: true
    schedule: "0 8 * * *"     # Mỗi ngày lúc 8:00 sáng
    timezone: "Asia/Ho_Chi_Minh"
```

### Nội Dung Báo Cáo

- **Nodes status**: ✅ 3/3 Ready hoặc 🔴 2/5 Ready
- **Pods summary**: Running, Failed, Pending counts
- **Resource usage**: CPU và RAM trung bình
- **Activity summary**: Tổng actions, restarts, scales, deploys trong 24h

### Tính Năng
- **Hỗ trợ timezone** (cron time)
- **Nhiều chat**: Gửi đến nhiều group/user cùng lúc
- **Chỉ chạy trên Leader** (tránh gửi trùng)

---

## Module Approval

Quy trình phê duyệt cho các thao tác nhạy cảm (sync production, rollback, scale).

### Tính Năng Chi Tiết

- **Rule-based**: Cấu hình actions và clusters cần approval
- **Multi-approval**: Yêu cầu N người phê duyệt (vd: 2/3 admins)
- **Approver roles**: Chỉ roles cụ thể mới được approve
- **Self-approval prevention**: Người tạo request không được tự approve
- **Expiry**: Request tự hết hạn (mặc định 30 phút)
- **Cancel**: Người tạo có thể hủy request pending
- **Background expiry worker**: Tự động đánh dấu expired

### Flow

```
Operator → /sync prod-app
         → "Cần 2 admin approval"
         → Admin A bấm ✅ Approve
         → Admin B bấm ✅ Approve
         → Tự động thực hiện sync
```

---

## Module Incident

Xây dựng incident timeline — tổng hợp K8s events và audit log theo thời gian.

### Lệnh

| Lệnh | Mô Tả |
|-------|--------|
| `/incident` | Mở incident timeline builder |

### Tính Năng

- **Chọn namespace** và **time window** (15m, 30m, 1h, 4h)
- **Kết hợp 2 nguồn dữ liệu**:
  - Kubernetes Events (⚠️ Warning, 📦 Scheduled, 💥 OOMKilling, 🔁 BackOff)
  - Audit Log (👤 User actions: scale, restart, deploy)
- **Sắp xếp theo thời gian** — xem diễn biến sự cố theo trình tự
- **Giới hạn 30 events** hiển thị (tránh quá dài)

---

## Module AlertManager

Nhận webhook từ Prometheus AlertManager và gửi cảnh báo qua Telegram.

### Tính Năng

- **Webhook receiver**: HTTP endpoint nhận alerts từ Prometheus
- **Bearer token auth**: Xác thực webhook requests
- **Format alerts**: Hiển thị severity, namespace, pod, summary, description
- **Silence từ Telegram**: Bấm nút 🔇 Silence 1h / 4h → tạo silence trên AlertManager API
- **Acknowledge**: Bấm ✅ Ack để ghi nhận
- **Critical first**: Sắp xếp alerts theo severity (critical → warning → info)
- **Resolved notifications**: Thông báo khi alert được giải quyết

---

## Module Quản Lý RBAC

Lệnh Telegram dành cho admin (`/rbac`) để quản lý roles người dùng tương tác.

### Tính Năng

- **Liệt kê users**: Xem tất cả users đã đăng ký với roles được gán
- **Liệt kê roles**: Xem 5 roles mặc định và custom roles với permissions
- **Gán role**: Chọn user, gán role mới qua inline buttons
- **Thu hồi role/binding**: Thu hồi dynamic role bindings hoặc reset về viewer
- **Chi tiết user**: Xem flat role, dynamic bindings, trạng thái super-admin
- **Bảo vệ quyền**: Tất cả actions yêu cầu quyền `admin.rbac.manage`
- **Audit đầy đủ**: Mọi thay đổi role đều được ghi vào audit log

---

## Module Notification Preferences

Cài đặt thông báo cá nhân qua lệnh `/notify` trên Telegram.

### Tính Năng

- **Lọc theo severity**: Chọn mức severity tối thiểu (`info`, `warning`, `critical`)
- **Giờ im lặng**: Tạm dừng cảnh báo không critical trong khung giờ
  - Tùy chọn: 22:00–08:00, 23:00–07:00, 00:00–06:00, hoặc tắt
- **Tắt cluster**: Tắt/bật notifications cho clusters cụ thể
- **Cài đặt riêng**: Mỗi user cài đặt độc lập
- **Lưu trữ bền**: Settings lưu trong SQLite/PostgreSQL
- **Tích hợp watcher**: Hàm `ShouldNotify()` để watchers kiểm tra trước khi gửi

---

## Hệ Thống RBAC

Role-Based Access Control — kiểm soát quyền truy cập chi tiết.

### Roles Mặc Định

| Role | Quyền |
|------|-------|
| **viewer** | Xem pods, logs, events, metrics, nodes (read-only) |
| **operator** | Viewer + restart pods, scale deployments, xem ArgoCD diff, Helm list |
| **admin** | Operator + manage nodes (cordon/drain), rollback Helm, manage freeze, sync/rollback ArgoCD, quản lý RBAC |
| **on-call** | Operator + rollback + drain (tự hết hạn) |
| **super-admin** | Full access — không giới hạn |

### Tính Năng Nâng Cao

- **Super Admin bypass**: Admin IDs trong config luôn có full quyền
- **Phase 4 Policy Rules**: Fine-grained RBAC với allow/deny rules
  - Match theo module, resource, action, cluster, namespace
  - **Deny overrides allow**
  - Hỗ trợ wildcard `*`
- **Custom Roles**: Tạo role tùy chỉnh
- **Role Bindings**: Gán role cho user, hỗ trợ expiry
- **Fallback**: Nếu không có dynamic binding → dùng legacy flat role
- **Quản lý trên Telegram**: Lệnh `/rbac` cho admin

---

## Hệ Thống Audit

Ghi log đầy đủ tất cả thao tác của users.

### Tính Năng

- **Buffered async writes**: Ghi log non-blocking qua channel (buffer 1000 entries)
- **Batch flush**: Ghi vào DB theo batch, tối ưu performance
- **Structured entries**: User, action, resource, cluster, namespace, status, details
- **Query API**: Tìm kiếm audit log với filter (page, page_size, user, action, cluster)
- **Middleware auto-log**: Tự động ghi audit cho mọi command
- **Concurrent safe**: Thread-safe cho nhiều goroutines ghi đồng thời

---

## Leader Election

Sử dụng Kubernetes Lease để bầu leader — đảm bảo chỉ 1 replica chạy watchers và briefing.

### Cách Hoạt Động

```
Pod A: Leader ← Chạy watchers, briefing scheduler
Pod B: Follower → Standby, sẵn sàng tiếp quản
Pod C: Follower → Standby
```

- **Lease-based**: Dùng K8s Lease object
- **Auto-recovery**: Khi leader mất, follower tự động tiếp quản
- **Callbacks**: `OnStartedLeading` → start watchers; `OnStoppedLeading` → stop watchers

---

## Multi-Cluster

Hỗ trợ quản lý nhiều Kubernetes clusters cùng lúc.

### Tính Năng

- **Cluster selector**: Bấm nút chọn cluster khi `/start`
- **Per-user context**: Mỗi user có cluster riêng (user A dùng prod, user B dùng staging)
- **Display name**: Tên hiển thị khác tên kỹ thuật (`Production Cluster` vs `prod-1`)
- **Health check**: Kiểm tra kết nối cluster định kỳ
- **Lazy initialization**: Client chỉ được tạo khi cần

---

## Bảo Mật

### Xác Thực & Phân Quyền
- **Telegram user ID**: Xác thực dựa trên Telegram user ID
- **RBAC per-command**: Mỗi lệnh yêu cầu quyền cụ thể
- **Admin IDs**: Danh sách super admins trong config
- **Allowed chats**: Giới hạn bot chỉ hoạt động trong chats được phép

### Rate Limiting
- **Per-user**: Giới hạn số lệnh mỗi user (mặc định 60/phút)
- **Sliding window**: Sử dụng thuật toán sliding window

### Middleware Chain
```
Request → Recovery → Rate Limit → Auth → Audit → Handler
```

---

## Observability

### Health & Readiness
- `/healthz` — Kiểm tra bot đang chạy
- `/readyz` — Kiểm tra dependencies (DB, clusters)

### Structured Logging
- **Zap JSON logger** với trace correlation
- **Log levels**: Debug, Info, Warn, Error
- **Sensitive data redaction**: Không log tokens, passwords

### Module Health Status
- Mỗi module báo cáo health: `healthy` / `unhealthy` / `unknown`
- Tổng hợp health từ tất cả modules

---

## Storage Backends

| Backend | Sử Dụng Cho | Phù Hợp |
|---------|-------------|---------|
| **SQLite** | Users, RBAC, Audit, Approvals, Freezes, Notification Prefs | Development, single instance |
| **PostgreSQL** | Giống SQLite nhưng scalable | Production, multi-instance |
| **Redis** | Cache, rate limiting, idempotency | Tất cả environments |

---

## Quốc Tế Hóa (i18n)

- **Hỗ trợ đa ngôn ngữ** qua package `pkg/i18n`
- **Messages tiếng Việt** cho briefing (vd: "Chúc team ngày mới ☀️")

---

*Telekube — Kubernetes management, right in your chat. 🚀*
