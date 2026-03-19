# Phần 8 — RBAC & Phân Quyền

> **Mục tiêu:** Hiểu hệ thống phân quyền (RBAC), cách gán vai trò cho người dùng, và cấu hình approval workflow cho các thao tác quan trọng.

---

## 8.1 Hệ thống vai trò (Roles)

Telekube có **5 vai trò** tích hợp sẵn:

| Vai trò | Mức độ truy cập |
|---------|----------------|
| **viewer** | Chỉ đọc — xem pod, log, events, metrics |
| **operator** | viewer + thao tác vận hành (restart, scale, sync diff) |
| **admin** | operator + quản trị (audit, RBAC, freeze, sync, rollback) |
| **on-call** | operator + rollback + drain (auto-expires) |
| **super-admin** | Toàn quyền — không bị giới hạn |

---

## 8.2 Phân quyền chi tiết

### Viewer

| Quyền | Mô tả |
|-------|-------|
| Pod list, get | Xem danh sách pod, chi tiết pod |
| Pod logs | Xem logs |
| Pod events | Xem K8s events |
| Metrics view | Xem CPU/RAM (`/top`) |
| Node view | Xem danh sách node |
| Quota view | Xem resource quota |
| ArgoCD apps list/view | Xem danh sách và chi tiết app |

### Operator (thêm so với viewer)

| Quyền | Mô tả |
|-------|-------|
| Pod restart | Restart pod |
| Deployment scale | Scale deployment/statefulset |
| ArgoCD diff | Xem diff trước khi sync |
| Helm releases list | Xem danh sách Helm release |

### Admin (thêm so với operator)

| Quyền | Mô tả |
|-------|-------|
| Pod delete | Xoá pod |
| Node cordon | Cordon/uncordon node |
| Node drain | Drain node |
| ArgoCD sync | Force sync ArgoCD app |
| ArgoCD rollback | Rollback ArgoCD app |
| ArgoCD freeze manage | Tạo/xoá deployment freeze |
| Helm rollback | Rollback Helm release |
| Admin users manage | Quản lý người dùng |
| Admin RBAC manage | Quản lý RBAC |
| Admin audit view | Xem audit log |

### On-Call (thêm so với operator)

| Quyền | Mô tả |
|-------|-------|
| ArgoCD rollback | Rollback khẩn trong sự cố |
| Node drain | Di tản workload khẩn |

> **Lưu ý:** Role `on-call` được thiết kế cho kỹ sư trực ca khẩn. Có thể cấu hình auto-expire sau một khoảng thời gian.

---

## 8.3 Bot Admin

Người dùng được liệt kê trong `telegram.admin_ids` trong config file **luôn có quyền admin** và không thể bị giảm quyền qua bot.

```yaml
telegram:
  admin_ids: [123456789, 987654321]
```

Đây là cơ chế bootstrap — dùng để gán quyền cho những người dùng khác.

---

## 8.4 Gán vai trò người dùng

### Xem vai trò hiện tại

```
/rbac
```

Bot admin có thể dùng lệnh `/rbac` để:
- Xem danh sách người dùng và vai trò
- Gán vai trò cho người dùng

### Gán vai trò

**Phân quyền tối thiểu:** `admin`

Workflow trong Telegram:

1. Bot admin gửi `/rbac`
2. Chọn người dùng cần cấp quyền
3. Chọn vai trò mới
4. Xác nhận

Hoặc thêm thủ công vào database (dùng cho bootstrap):

```sql
-- SQLite
INSERT INTO users (telegram_id, username, role, created_at, updated_at)
VALUES (987654321, 'john_doe', 'operator', datetime('now'), datetime('now'));
```

### Tự đăng ký (nếu cấu hình cho phép)

Người dùng mới tự động nhận role `viewer` theo cấu hình:

```yaml
rbac:
  default_role: viewer  # hoặc "operator" nếu muốn mở hơn
```

---

## 8.5 Approval Workflow

Khi bật module approval, một số thao tác nhạy cảm **yêu cầu được phê duyệt** trước khi thực hiện.

### Kích hoạt

```yaml
modules:
  approval:
    enabled: true
```

### Cách hoạt động

```
Operator gửi yêu cầu sync production    
         ↓
Hệ thống tạo ApprovalRequest (ID: req-01JXYZ)
         ↓
Thông báo gửi đến admin chat:
  ┌──────────────────────────────────┐
  │ 🔔 Approval Request              │
  │                                  │
  │ From: @john_operator             │
  │ Action: argocd.apps.sync         │
  │ App: backend-api (production)    │
  │ Time: 14:00 UTC                  │
  │ Expires in: 30 phút              │
  │                                  │
  │ [✅ Approve] [❌ Reject]          │
  └──────────────────────────────────┘
         ↓
Admin nhấn Approve / Reject
         ↓
Nếu Approved → Hệ thống tự động thực hiện sync
Nếu Rejected → Operator nhận thông báo từ chối
```

### Quy tắc (Rules)

Approval rules được cấu hình theo format:

```go
// Cấu hình programmatic (trong code/config)
Rules: []Rule{
    {
        Action:            "argocd.apps.sync",
        Clusters:          []string{"production"},
        RequiredApprovals: 1,
        ApproverRoles:     []string{"admin", "super-admin"},
    },
    {
        Action:            "argocd.apps.rollback",
        Clusters:          []string{"*"},   // Tất cả cluster
        RequiredApprovals: 1,
        ApproverRoles:     []string{"admin"},
    },
}
```

### Xem yêu cầu đang chờ

Các yêu cầu phê duyệt đang chờ xuất hiện dưới dạng tin nhắn tương tác trong admin chat với các nút **Approve / Reject / Cancel** dạng inline button. Không có lệnh `/approve` — việc phê duyệt được thực hiện hoàn toàn qua inline button khi có request được gửi lên.

**Phân quyền tối thiểu để phê duyệt/từ chối:** `admin`

### Các trạng thái approval

| Trạng thái | Mô tả |
|-----------|-------|
| `pending` | Đang chờ phê duyệt |
| `approved` | Đã được phê duyệt, hành động đang thực hiện |
| `rejected` | Bị từ chối |
| `expired` | Hết hạn (mặc định 30 phút) |
| `cancelled` | Người đề xuất hủy |

### Tự phê duyệt (không được phép)

Hệ thống ngăn người đề xuất tự phê duyệt yêu cầu của chính mình:

```
❌ Bạn không thể tự phê duyệt yêu cầu của mình.
Vui lòng nhờ admin khác phê duyệt.
```

---

## 8.6 Vai trò On-Call

Role `on-call` phù hợp cho kỹ sư trực ca khẩn vào ban đêm/cuối tuần:

**Quyền hạn:**
- Tất cả quyền của `operator`
- Rollback ArgoCD app (không cần approval)
- Drain node (trong trường hợp khẩn)

**Cách dùng điển hình:**

```
1. Sự cố production lúc 2 giờ sáng
2. Kỹ sư on-call cần rollback gấp
3. Admin tạm thời gán role "on-call" cho kỹ sư qua /rbac
4. Kỹ sư thực hiện rollback tự chủ
5. Sau khi xử lý xong, admin thu hồi role "on-call"
```

---

## 8.7 Bảng ma trận phân quyền hoàn chỉnh

| Hành động | viewer | operator | on-call | admin | super-admin |
|-----------|:------:|:--------:|:-------:|:-----:|:-----------:|
| Xem pod | ✅ | ✅ | ✅ | ✅ | ✅ |
| Xem logs | ✅ | ✅ | ✅ | ✅ | ✅ |
| Xem metrics (`/top`) | ✅ | ✅ | ✅ | ✅ | ✅ |
| Xem node | ✅ | ✅ | ✅ | ✅ | ✅ |
| Xem ArgoCD apps | ✅ | ✅ | ✅ | ✅ | ✅ |
| Restart pod | — | ✅ | ✅ | ✅ | ✅ |
| Scale deployment | — | ✅ | ✅ | ✅ | ✅ |
| ArgoCD diff | — | ✅ | ✅ | ✅ | ✅ |
| Xem Helm releases | — | ✅ | ✅ | ✅ | ✅ |
| Xoá pod | — | — | — | ✅ | ✅ |
| Cordon/Uncordon node | — | — | — | ✅ | ✅ |
| Drain node | — | — | ✅ | ✅ | ✅ |
| ArgoCD sync | — | — | — | ✅ | ✅ |
| ArgoCD rollback | — | — | ✅ | ✅ | ✅ |
| Deployment freeze | — | — | — | ✅ | ✅ |
| Helm rollback | — | — | — | ✅ | ✅ |
| Phê duyệt approval | — | — | — | ✅ | ✅ |
| Xem audit log | — | — | — | ✅ | ✅ |
| Quản lý RBAC | — | — | — | ✅ | ✅ |

---

## Bước tiếp theo

- [Audit Log →](09-audit.md)
- [Tham chiếu lệnh →](10-lenh.md)
