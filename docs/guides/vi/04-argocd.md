# Phần 4 — ArgoCD & GitOps

> **Mục tiêu:** Quản lý ứng dụng ArgoCD, thực hiện sync, rollback, xem dashboard, và kiểm soát deployment thông qua Deployment Freeze.

---

## 4.1 Yêu cầu & Kích hoạt

### Kích hoạt module

```yaml
modules:
  argocd:
    enabled: true

argocd:
  insecure: false
  timeout: 30s
  instances:
    - name: "prod-argocd"
      url: "https://argocd.example.com"
      auth:
        type: "token"
        token: "${ARGOCD_TOKEN}"
      clusters:
        - "production"
```

### Lấy ArgoCD Token

```bash
# Đăng nhập ArgoCD CLI
argocd login argocd.example.com --username admin

# Tạo token cho Telekube
argocd account generate-token --account telekube
```

Lưu token vào biến môi trường:

```bash
export ARGOCD_TOKEN="eyJhbGciOiJIUzI1NiI..."
```

---

## 4.2 `/apps` — Danh sách ứng dụng

```
/apps
```

### Cách sử dụng

1. Gửi `/apps`
2. Nếu có nhiều ArgoCD instance, bot hỏi chọn instance
3. Bot liệt kê ứng dụng với trạng thái:

```
✅ backend-api        Synced    Healthy
🟡 frontend           OutOfSync Healthy
🔴 worker-service     Synced    Degraded
⚪ batch-job          Synced    Progressing
```

**Ký hiệu trạng thái:**

| Ký hiệu | Sync Status | Health Status |
|---------|-------------|---------------|
| ✅ | Synced | Healthy |
| 🟡 | OutOfSync | — |
| 🔴 | — | Degraded/Missing |
| ⚪ | — | Progressing/Unknown |

4. Nhấn vào tên ứng dụng để xem chi tiết

### Chi tiết ứng dụng

Màn hình chi tiết hiển thị:
- **Git repo** và **branch/revision** hiện tại
- **Helm chart** (nếu dùng Helm)
- Danh sách **resources** (Deployment, Service, Ingress...) với trạng thái từng resource
- Lịch sử sync gần đây

Từ màn hình chi tiết, có thể:
- **Sync** — đồng bộ lại với Git
- **Rollback** — quay về revision cũ
- **Back** — quay lại danh sách

**Phân quyền tối thiểu:** `viewer`

---

## 4.3 Sync ứng dụng

### Sync thông thường

1. Nhấn vào ứng dụng → **Sync**
2. Bot hiển thị menu tuỳ chọn sync:
   - **Sync Now** — sync bình thường
   - **Sync + Prune** — sync và xoá resource đã bị xoá khỏi Git
   - **Force Sync** — force overwrite local state
3. Xác nhận
4. Bot thực hiện sync và thông báo kết quả

> **Lưu ý:** Thao tác Sync yêu cầu quyền `admin` theo mặc định. Xem [phân quyền ArgoCD](08-rbac.md#argocd).

**Phân quyền tối thiểu:** `admin`

### Xem Diff trước khi Sync

Trước khi sync, người dùng `operator` có thể xem diff:

1. Nhấn vào ứng dụng
2. Nhấn **Diff** (nếu ứng dụng OutOfSync)
3. Bot hiển thị những gì sẽ thay đổi khi sync

**Phân quyền tối thiểu:** `operator`

---

## 4.4 Rollback ứng dụng

Rollback đưa ứng dụng về một revision cũ trong lịch sử ArgoCD:

1. Nhấn vào ứng dụng → **Rollback**
2. Bot liệt kê các revision trong lịch sử 10 lần sync gần nhất:
   ```
   Rev 42 — commit abc1234 (2h ago) — Healthy
   Rev 41 — commit def5678 (1d ago) — Healthy
   Rev 40 — commit ghi9012 (3d ago) — Degraded
   ```
3. Chọn revision muốn rollback về
4. Xác nhận
5. Bot thực hiện rollback

**Phân quyền tối thiểu:** `admin`

> **Khẩn cấp:** Trong tình huống sự cố production, on-call engineer với role `on-call` cũng có quyền rollback.

---

## 4.5 `/dashboard` — Dashboard GitOps

```
/dashboard
```

Dashboard tổng hợp hiển thị tình trạng tất cả ứng dụng ArgoCD:

```
GitOps Dashboard — prod-argocd
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Total:      24 apps
Healthy:    21
Degraded:    2
OutOfSync:   1

✅ 21 Synced & Healthy
🟡  1 Out of Sync
🔴  2 Degraded
```

Từ dashboard có thể:
- **Refresh** — cập nhật trạng thái
- **Lọc Out of Sync** — xem chỉ app bị OutOfSync
- **Lọc Degraded** — xem chỉ app đang lỗi

**Phân quyền tối thiểu:** `viewer`

---

## 4.6 `/freeze` — Deployment Freeze

Deployment Freeze là tính năng **chặn mọi thao tác sync và rollback** trong một khoảng thời gian nhất định. Dùng trong các trường hợp:

- Maintenance window
- Trước/trong sự kiện lớn (Black Friday, launch...)
- Điều tra sự cố production

---

### Tạo Freeze

1. Gửi `/freeze`
2. Bot hiển thị menu:
   - **Tạo freeze mới**
   - **Xem lịch sử freeze**
   - **Xoá freeze đang hoạt động**
3. Chọn **Tạo freeze mới**
4. Chọn **phạm vi**:
   - `Tất cả ứng dụng` — chặn mọi sync/rollback
   - `Namespace cụ thể` — chỉ chặn trong namespace được chọn
5. Chọn **thời hạn**:
   - 1 giờ / 2 giờ / 4 giờ / 8 giờ / 24 giờ
6. Xác nhận — Freeze có hiệu lực ngay lập tức

**Kết quả:** Khi người dùng cố sync/rollback trong khi đang freeze:

```
🧊 Deployment Freeze đang hoạt động!

Phạm vi: Tất cả ứng dụng
Thời gian: 14:00 → 18:00 (còn 2h30m)
Lý do: Maintenance window

Liên hệ admin để thực hiện deploy trong trường hợp khẩn cấp.
```

**Phân quyền tối thiểu:** `admin`

---

### Xoá Freeze (Thaw)

1. Gửi `/freeze`
2. Chọn **Xoá freeze hiện tại** (Thaw)
3. Xác nhận

---

### Xem lịch sử Freeze

1. Gửi `/freeze`
2. Chọn **Lịch sử**
3. Bot hiển thị danh sách freeze trong 30 ngày qua

---

## 4.7 Sync/Rollback với Approval Workflow

Khi bật module `approval` và có cấu hình approval cho ArgoCD, các thao tác sync/rollback trên cluster production sẽ yêu cầu phê duyệt:

1. Người dùng `operator` gửi yêu cầu sync
2. Hệ thống tạo approval request và gửi thông báo đến admin
3. Admin nhấn **Approve** hoặc **Reject**
4. Nếu được duyệt, sync được thực hiện tự động

Xem chi tiết tại [Phần 8 — RBAC & Approval](08-rbac.md).

---

## 4.8 Bảng tóm tắt lệnh ArgoCD

| Lệnh / Hành động | Mô tả | Phân quyền |
|-----------------|-------|-----------|
| `/apps` | Danh sách ứng dụng | viewer |
| `/dashboard` | GitOps dashboard | viewer |
| Diff ứng dụng | Xem thay đổi sẽ được deploy | operator |
| Sync ứng dụng | Đồng bộ với Git | admin |
| Rollback ứng dụng | Quay về revision cũ | admin |
| `/freeze` | Tạo/xoá deployment freeze | admin |

---

## Bước tiếp theo

- [Helm Management →](05-helm.md)
- [Giám sát & Cảnh báo →](06-watcher.md)
- [RBAC & Approval →](08-rbac.md)
