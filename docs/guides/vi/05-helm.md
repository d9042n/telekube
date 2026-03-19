# Phần 5 — Helm

> **Mục tiêu:** Xem danh sách Helm release, kiểm tra chi tiết và rollback về revision cũ trực tiếp từ Telegram.

---

## 5.1 Yêu cầu & Kích hoạt

```yaml
modules:
  helm:
    enabled: true
```

Helm module sử dụng Kubernetes REST config để truy cập vào Helm secrets — không cần cài thêm Helm CLI trên server chạy Telekube.

---

## 5.2 `/helm` — Danh sách Helm Release

```
/helm
```

### Cách sử dụng

1. Gửi `/helm`
2. Bot hỏi chọn namespace:
   - `[All Namespaces]` — xem tất cả namespace
   - `production`
   - `staging`
   - `kube-system`
3. Bot trả về danh sách release với thông tin:

```
⎈ Helm Releases — production (cluster: my-cluster)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ backend-api         1.4.2    deployed   Rev 8   2h ago
✅ frontend            2.1.0    deployed   Rev 12  1d ago
🔴 worker-service      3.0.1    failed     Rev 5   30m ago
🟡 batch-job           1.0.0    pending    Rev 2   5m ago
```

**Trạng thái release:**

| Ký hiệu | Trạng thái Helm |
|---------|----------------|
| ✅ | deployed — đang chạy bình thường |
| 🔴 | failed — deploy thất bại |
| 🟡 | pending-install / pending-upgrade / pending-rollback |
| ⚪ | Trạng thái khác |

4. Nhấn vào tên release để xem chi tiết

**Phân quyền tối thiểu:** `operator` (xem danh sách)

---

## 5.3 Chi tiết Release

Màn hình chi tiết hiển thị:

```
⎈ backend-api (production)
━━━━━━━━━━━━━━━━━━━━━━
Chart:    backend-chart-0.9.3
App:      1.4.2
Status:   deployed
Revision: 8
Updated:  2026-03-18 06:00 UTC

History:
  Rev 8  — 1.4.2    (current)
  Rev 7  — 1.4.1
  Rev 6  — 1.4.0
  Rev 5  — 1.3.9
  Rev 4  — 1.3.8
```

Từ màn hình chi tiết:
- **Rollback** — chọn revision cũ để rollback về
- **Back** — quay lại danh sách namespace

---

## 5.4 Rollback Helm Release

Rollback đưa release về một revision cụ thể:

1. Nhấn vào release → **Rollback**
2. Bot liệt kê lịch sử revision (tối đa 10 gần nhất):
   ```
   Chọn revision để rollback:
   ─────────────────────────
   Rev 7 — 1.4.1
   Rev 6 — 1.4.0
   Rev 5 — 1.3.9
   ```
3. Chọn revision muốn rollback về
4. Bot xác nhận và thực hiện rollback (timeout 5 phút)
5. Bot thông báo kết quả:
   ```
   ✅ Rollback hoàn tất! backend-api hiện ở Rev 9 (rollback về 1.4.1)
   ```

> **Ghi chú:** Mỗi lần rollback tạo ra một revision mới. Ví dụ: rollback từ Rev 8 về Rev 7 sẽ tạo Rev 9 với chart giống Rev 7.

**Phân quyền tối thiểu:** `admin` (cho rollback)

---

## 5.5 Refresh

Nhấn nút **Refresh** để cập nhật danh sách release (nếu có release mới được deploy).

---

## 5.6 Bảng tóm tắt lệnh Helm

| Hành động | Mô tả | Phân quyền |
|-----------|-------|-----------|
| `/helm` | Xem danh sách release | operator |
| Xem chi tiết release | Chart, version, history | operator |
| Rollback release | Quay về revision cũ | admin |

---

## 5.7 Lưu ý

- Telekube chỉ hỗ trợ **xem** và **rollback** — không hỗ trợ install hoặc upgrade release thông qua Telegram.
- Để deploy Helm chart mới hoặc upgrade, sử dụng ArgoCD (GitOps) hoặc Helm CLI.
- History chỉ hiển thị tối đa **10 revision** gần nhất.

---

## Bước tiếp theo

- [Giám sát & Cảnh báo →](06-watcher.md)
- [Incident & Briefing →](07-incident-briefing.md)
