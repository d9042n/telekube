# Phần 7 — Incident & Briefing

> **Mục tiêu:** Xây dựng timeline sự cố tự động từ Kubernetes events và audit log; nhận báo cáo sức khoẻ cluster hằng ngày vào Telegram.

---

## 7.1 Incident Timeline

### Giới thiệu

Module Incident giúp bạn nhanh chóng hiểu **điều gì đã xảy ra** trong cluster trong một khoảng thời gian nhất định bằng cách tổng hợp:

- **Kubernetes Events:** Pod crash, container restart, scheduler events, OOMKill...
- **Audit Log Telekube:** Hành động của người dùng qua bot (restart, scale, sync...)

### Kích hoạt

```yaml
modules:
  incident:
    enabled: true
```

---

### 7.1.1 `/incident` — Tạo timeline

```
/incident
```

**Cách sử dụng:**

1. Gửi `/incident`
2. Chọn **namespace** muốn điều tra (hoặc `All Namespaces`)
3. Chọn **cửa sổ thời gian:**
   - `⏱️ 15 phút gần nhất`
   - `⏱️ 30 phút gần nhất`
   - `⏱️ 1 giờ gần nhất`
   - `⏱️ 4 giờ gần nhất`
4. Bot xây dựng và gửi timeline

**Ví dụ kết quả:**

```
🚨 Incident Timeline — production (1 giờ gần nhất)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Cluster: prod-us-east | 13:00 — 14:00 UTC

14:00  💥 api-server-7d8f9b — OOMKilling (container api used 512Mi)
13:58  ⚠️ api-server-7d8f9b — BackOff (Back-off restarting failed container)
13:57  👤 @ops-john — kubernetes.pods.restart api-server-7d8f9b
13:55  ↙️ api-server-7d8f9b — Started
13:54  📦 api-server-7d8f9b — Pulled (Successfully pulled image v1.4.2)
13:50  ⚠️ worker-node-3 — NodeNotReady
13:45  👤 @admin-alice — kubernetes.nodes.drain worker-node-3
13:40  📋 batch-job-28501000 — Scheduled (Pod assigned to worker-node-4)

═══════════════════════════════════════════
Total events: 8
```

**Ký hiệu trong timeline:**

| Ký hiệu | Loại sự kiện |
|---------|-------------|
| 💥 | OOMKill |
| ⚠️ | Warning / NodeNotReady |
| 👤 | Hành động người dùng (audit) |
| ▶️ | Container started |
| 📦 | Image pulled / Pod scheduled |
| 🔁 | BackOff / Restart |
| 📋 | Sự kiện Kubernetes thông thường |

**Từ màn hình timeline:**
- **Refresh** — cập nhật timeline
- **Back** — chọn lại namespace

**Phân quyền tối thiểu:** `viewer`

> **Lưu ý về giới hạn:** Nếu có hơn 30 events, bot hiển thị "Showing first 30 of X events" và danh sách rút ngắn. Truy cập trực tiếp vào Kubernetes để xem toàn bộ.

---

## 7.2 Daily Briefing (Báo cáo hằng ngày)

### Giới thiệu

Module Briefing gửi **báo cáo tổng hợp sức khoẻ cluster** vào một giờ cố định mỗi ngày. Giúp team nắm bắt tình trạng hệ thống mà không cần chủ động kiểm tra.

### Kích hoạt

```yaml
modules:
  briefing:
    enabled: true
    schedule: "0 8 * * *"           # Mỗi ngày 8:00 sáng
    timezone: "Asia/Ho_Chi_Minh"    # Múi giờ Việt Nam (UTC+7)
```

---

### 7.2.1 Nội dung báo cáo

Báo cáo được gửi tự động đến tất cả `allowed_chats`:

```
📊 Daily Cluster Briefing
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Thứ Ba, 18/03/2026 — 08:00 ICT

Cluster: production (prod-us-east)
━━━━━━━━━━━━━━━━

NODES (5/5 healthy)
✅ worker-node-1   Ready   CPU: 45%   RAM: 62%
✅ worker-node-2   Ready   CPU: 38%   RAM: 58%
✅ worker-node-3   Ready   CPU: 71%   RAM: 75%
✅ worker-node-4   Ready   CPU: 29%   RAM: 41%
✅ control-plane   Ready   CPU: 12%   RAM: 35%

PODS
Total: 124 | Running: 121 | Pending: 1 | Failed: 2

⚠️ Pods cần chú ý:
  🔴 batch-worker-2  — CrashLoopBackOff (Namespace: batch)
  🔴 report-gen-old  — OOMKilled         (Namespace: reports)
  🟡 cache-warmup    — Pending 8m        (Namespace: backend)

DEPLOYMENTS (48 total)
✅ 46 Available    🟡 2 Degraded

ArgoCD (prod-argocd)
✅ 21 Synced    🟡 1 OutOfSync    🔴 2 Degraded

─────────────────────────────
📅 Hôm qua: 3 incidents, 7 user actions
🕐 24h alert count: 5 warnings, 1 critical
```

---

### 7.2.2 Tuỳ chỉnh lịch

Ví dụ các schedule phổ biến:

```yaml
# 8 giờ sáng mỗi ngày
schedule: "0 8 * * *"

# 8 giờ sáng các ngày trong tuần (Thứ 2 - Thứ 6)
schedule: "0 8 * * 1-5"

# Hai lần/ngày: 8 giờ sáng và 5 giờ chiều
schedule: "0 8,17 * * *"

# 9 giờ sáng mỗi thứ Hai đầu tuần
schedule: "0 9 * * 1"
```

> **Cú pháp cron:** `<phút> <giờ> <ngày-tháng> <tháng> <thứ>`

---

### 7.2.3 Múi giờ

Bot tự động chuyển đổi thời gian sang múi giờ được cấu hình. Sử dụng tên múi giờ theo chuẩn [IANA Time Zone Database](https://www.iana.org/time-zones):

| Khu vực | Tên IANA |
|---------|----------|
| Hà Nội / TP.HCM (UTC+7) | `Asia/Ho_Chi_Minh` |
| Bangkok (UTC+7) | `Asia/Bangkok` |
| Singapore (UTC+8) | `Asia/Singapore` |
| Seoul / Tokyo (UTC+9) | `Asia/Seoul` |
| London (GMT/BST) | `Europe/London` |
| New York (EST/EDT) | `America/New_York` |
| UTC | `UTC` |

---

## 7.3 Bảng tóm tắt

| Tính năng | Lệnh / Trigger | Phân quyền |
|-----------|---------------|-----------|
| Incident Timeline | `/incident` | viewer |
| Daily Briefing | Tự động theo schedule | — (tất cả `allowed_chats`) |

---

## Bước tiếp theo

- [RBAC & Phân quyền →](08-rbac.md)
- [Audit Log →](09-audit.md)
