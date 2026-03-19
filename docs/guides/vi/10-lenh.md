# Phần 10 — Tham Chiếu Lệnh

> Danh sách đầy đủ tất cả lệnh Telegram của Telekube theo nhóm chức năng.

---

## 10.1 Lệnh chung

| Lệnh | Mô tả | Phân quyền |
|------|-------|-----------|
| `/start` | Màn hình chào mừng + chọn cluster | Tất cả |
| `/help` | Danh sách lệnh khả dụng | Tất cả |
| `/clusters` | Chuyển đổi cluster đang làm việc | Tất cả |

---

## 10.2 Kubernetes — Đọc

| Lệnh | Mô tả | Phân quyền |
|------|-------|-----------|
| `/pods` | Danh sách pod (chọn namespace qua button) | viewer |
| `/pods <namespace>` | Danh sách pod trong namespace cụ thể | viewer |
| `/logs <pod>` | Xem 100 dòng log cuối của pod | viewer |
| `/events <pod>` | Xem Kubernetes events của pod | viewer |
| `/nodes` | Danh sách và trạng thái node | viewer |
| `/top` | Metrics CPU/RAM của pod (chọn namespace) | viewer |
| `/top nodes` | Metrics CPU/RAM của node | viewer |
| `/quota` | Resource quota theo namespace | viewer |
| `/namespaces` | Danh sách namespace với trạng thái | viewer |
| `/deploys [ns]` | Danh sách deployment | viewer |
| `/cronjobs [ns]` | Trạng thái CronJob | viewer |

---

## 10.3 Kubernetes — Thao tác

| Lệnh / Hành động | Mô tả | Phân quyền |
|-----------------|-------|-----------|
| `/restart <pod>` | Restart pod (xoá + controller tạo lại) | operator |
| Restart pod (từ pod detail) | Restart pod qua button | operator |
| `/scale` | Scale Deployment/StatefulSet | operator |
| Cordon node (từ node detail) | Ngăn lên lịch pod mới | admin |
| Uncordon node (từ node detail) | Mở lại lên lịch pod | admin |
| Drain node (từ node detail) | Di tản workload, chuẩn bị bảo trì node | admin |
| Xoá pod (từ pod detail) | Xoá pod vĩnh viễn | admin |

---

## 10.4 ArgoCD

| Lệnh / Hành động | Mô tả | Phân quyền |
|-----------------|-------|-----------|
| `/apps` | Danh sách ArgoCD app với trạng thái | viewer |
| `/dashboard` | GitOps dashboard tổng hợp | viewer |
| Xem diff (từ app detail) | Diff trước khi sync | operator |
| Sync app (từ app detail) | Force sync với Git | admin |
| Rollback app (từ app detail) | Rollback về revision cũ | admin |
| `/freeze` | Tạo / xoá Deployment Freeze | admin |

---

## 10.5 Helm

| Lệnh / Hành động | Mô tả | Phân quyền |
|-----------------|-------|-----------|
| `/helm` | Danh sách Helm release (chọn namespace) | operator |
| Xem chi tiết release | Chart, version, lịch sử revision | operator |
| Rollback release (từ release detail) | Rollback về revision Helm cụ thể | admin |

---

## 10.6 Giám sát & Cảnh báo

| Tính năng | Trigger | Phân quyền |
|-----------|---------|-----------|
| Pod crash alert | Tự động (watcher) | — |
| Node down alert | Tự động (watcher) | — |
| CronJob failed alert | Tự động (watcher) | — |
| Cert expiry alert | Tự động (watcher) | — |
| PVC usage alert | Tự động (watcher) | — |
| Custom alert rules | Tự động (evaluator, cấu hình trong config) | — |
| AlertManager webhook | Tự động (Prometheus) | — |
| Mute alert 1 giờ | Nhấn button trong alert | viewer |

---

## 10.7 Incident & Báo cáo

| Lệnh | Mô tả | Phân quyền |
|------|-------|-----------|
| `/incident` | Xây dựng timeline sự cố | viewer |
| Daily Briefing | Tự động theo lịch (cron) | — |

---

## 10.8 Quản trị

| Lệnh | Mô tả | Phân quyền |
|------|-------|-----------|
| `/audit` | Xem 20 hành động vận hành gần nhất | admin |
| `/rbac` | Quản lý vai trò và phân quyền người dùng | admin |
| `/notify` | Quản lý cài đặt thông báo (mức độ, giờ im lặng, tắt cluster) | Tất cả |

> `/approve` không phải là lệnh slash — phê duyệt được thực hiện qua inline button khi có approval request.

---

## 10.9 Bảng tổng hợp nhanh

```
VIEWER           OPERATOR              ADMIN
━━━━━━━━━━━━━━   ━━━━━━━━━━━━━━━━━━   ━━━━━━━━━━━━━━━━━━━━━━━━
/start           /start               /start
/help            /help                /help
/clusters        /clusters            /clusters
/pods            /pods                /pods
/logs            /logs                /logs
/events          /events              /events
/nodes           /nodes               /nodes
/top             /top                 /top
/quota           /quota               /quota
/apps            /apps                /apps
/dashboard       /dashboard           /dashboard
/incident        /incident            /incident
/notify          /notify              /notify
                 Restart pod          Restart pod
                 /scale               /scale
                 /helm                Cordon/Uncordon node
                                      Drain node
                                      Xoá pod
                                      Sync ArgoCD
                                      Rollback ArgoCD
                                      Helm rollback
                                      /freeze
                                      /audit
                                      /rbac
                                      Phê duyệt request
```

---

## 10.10 Phím tắt và Inline button

Hầu hết tính năng của Telekube sử dụng **inline keyboard button** thay vì phải nhớ cú pháp lệnh phức tạp.

**Luồng điển hình:**

```
/pods
  → Chọn namespace [button]
    → Danh sách pod
      → Nhấn pod name [button]  
        → Pod Detail
          → [Logs] [Events] [Restart] [Back]
```

```
/nodes
  → Danh sách node
    → Nhấn node name [button]
      → Node Detail
        → [Cordon] [Uncordon] [Drain] [Top Pods] [Back]
```

```
/apps
  → Chọn instance [button] (nếu nhiều instance)
    → Danh sách app
      → Nhấn app name [button]
        → App Detail
          → [Sync] [Diff] [Rollback] [Back]
```

---

## 10.11 Xử lý lỗi phổ biến

| Lỗi | Nguyên nhân | Giải pháp |
|-----|------------|-----------|
| `🚫 Insufficient permissions` | Không đủ quyền | Liên hệ admin để được cấp quyền |
| `No clusters configured` | Chưa cấu hình cluster | Kiểm tra `configs/config.yaml` |
| `metrics-server not available` | Chưa cài metrics-server | Cài metrics-server vào cluster |
| `ArgoCD instance not found` | Module ArgoCD chưa bật hoặc chưa cấu hình | Bật `modules.argocd.enabled: true` và thêm instance |
| `Deployment freeze active` | Đang trong thời gian freeze | Chờ freeze hết hạn hoặc liên hệ admin |
| `request expired` | Approval request quá 30 phút | Gửi yêu cầu mới |
| `Rate limit exceeded` | Gửi quá nhiều lệnh | Chờ 1 phút và thử lại |

---

## Tham Chiếu Thêm

- [Cài đặt & Bắt đầu](01-cai-dat.md)
- [Cấu hình](02-cau-hinh.md)
- [RBAC & Phân quyền](08-rbac.md)
- [Quay lại mục lục](README.md)
