# Phần 9 — Audit Log

> **Mục tiêu:** Xem và hiểu nhật ký hành động (audit log) — ai đã làm gì, lúc nào, trên cluster nào.

---

## 9.1 Giới thiệu

Telekube tự động ghi lại tất cả các hành động **vận hành** được thực hiện qua bot vào Audit Log. Mỗi entry bao gồm:

- **Người thực hiện** (Telegram username + ID)
- **Hành động** (restart, scale, sync, rollback...)
- **Resource bị tác động** (tên pod, deployment, app...)
- **Cluster và Namespace**
- **Thời điểm** (UTC)
- **Kết quả** (success / failed)

Audit log giúp trả lời câu hỏi: *"Ai đã restart pod này? Ai đã scale deployment kia? Ai đã sync ArgoCD lúc 2 giờ sáng?"*

---

## 9.2 `/audit` — Xem Audit Log

```
/audit
```

**Phân quyền tối thiểu:** `admin`

### Cách sử dụng

1. Gửi `/audit`
2. Bot trả về 20 hành động gần nhất (mới nhất đứng trên cùng):

```
📋 Audit Log — 20 entries
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

14:05 UTC  @john_ops       kubernetes.pods.restart
           api-server-7d8f9b    prod / backend    ✅

13:57 UTC  @alice_admin     argocd.apps.sync
           backend-api          prod-argocd       ✅

13:45 UTC  @alice_admin     kubernetes.nodes.drain
           worker-node-3        production        ✅

13:30 UTC  @bob_dev         kubernetes.deployments.scale
           frontend (3→5)       prod / frontend   ✅

12:10 UTC  @john_ops       kubernetes.pods.restart
           worker-svc-abc       prod / batch      ❌ (forbidden)

...
```

**Ký hiệu kết quả:**
- ✅ `success` — thao tác thành công
- ❌ `failed` — thao tác thất bại (lý do ghi trong chi tiết)
- 🚫 `forbidden` — không có quyền

---

## 9.3 Sử dụng Audit Log trong Incident

Audit log được tích hợp vào **Incident Timeline**. Khi bạn dùng `/incident`, các hành động của người dùng (👤) xuất hiện xen kẽ với Kubernetes events:

```
14:00  💥 api-server — OOMKilling
13:58  ⚠️ api-server — BackOff restart
13:57  👤 @ops-john — kubernetes.pods.restart api-server-7d8f9b   ← từ audit log
13:55  ▶️ api-server — Started
```

Điều này giúp phân tích **tương quan nhân quả** — xem hành động nào của người dùng liên quan đến sự kiện trong cluster.

---

## 9.4 Các hành động được ghi lại

| Hành động | Mô tả |
|-----------|-------|
| `kubernetes.pods.restart` | Restart pod |
| `kubernetes.pods.delete` | Xoá pod |
| `kubernetes.deployments.scale` | Scale deployment |
| `kubernetes.nodes.cordon` | Cordon node |
| `kubernetes.nodes.uncordon` | Uncordon node |
| `kubernetes.nodes.drain` | Drain node |
| `argocd.apps.sync` | Sync ArgoCD app |
| `argocd.apps.rollback` | Rollback ArgoCD app |
| `argocd.freeze.create` | Tạo deployment freeze |
| `argocd.freeze.thaw` | Xoá deployment freeze |
| `helm.releases.rollback` | Rollback Helm release |
| `admin.rbac.assign` | Gán vai trò người dùng |
| `admin.rbac.revoke` | Thu hồi vai trò |

---

## 9.5 Lưu trữ và phân trang

- Audit log được lưu trong database (SQLite **hoặc** PostgreSQL)
- Mặc định hiển thị 20 entries mới nhất
- Dữ liệu không tự động xoá — cần quản lý thủ công nếu dung lượng là vấn đề
- Để truy vấn nâng cao (lọc theo user, action, thời gian), truy cập trực tiếp database

### Truy vấn database trực tiếp

**SQLite:**

```bash
sqlite3 telekube.db

-- 50 actions của @john_ops
SELECT occurred_at, username, action, resource, cluster, namespace, status
FROM audit_log
WHERE username = 'john_ops'
ORDER BY occurred_at DESC
LIMIT 50;

-- Tất cả sync action trên production hôm nay
SELECT occurred_at, username, action, resource, status
FROM audit_log
WHERE action LIKE 'argocd.apps.sync'
  AND cluster = 'production'
  AND date(occurred_at) = date('now')
ORDER BY occurred_at DESC;
```

**PostgreSQL:**

```sql
-- Hành động trong 24 giờ qua
SELECT occurred_at, username, action, resource, cluster, namespace, status
FROM audit_log
WHERE occurred_at >= NOW() - INTERVAL '24 hours'
ORDER BY occurred_at DESC;
```

---

## 9.6 Bảng tóm tắt

| Tính năng | Mô tả | Phân quyền |
|-----------|-------|-----------|
| `/audit` | Xem 20 hành động gần nhất | admin |
| Audit trong `/incident` | Timeline kết hợp K8s events + audit | viewer |
| Truy vấn database | Tìm kiếm nâng cao | Admin hệ thống |

---

## Bước tiếp theo

- [Tham chiếu lệnh đầy đủ →](10-lenh.md)
- [Quay lại mục lục →](README.md)
