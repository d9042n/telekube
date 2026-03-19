# Hướng Dẫn Sử Dụng Telekube (Tiếng Việt)

> **Telekube** — Trung tâm điều khiển Kubernetes & ArgoCD qua Telegram

---

## Mục Lục

| # | Tài liệu | Mô tả |
|---|----------|-------|
| 1 | [Cài đặt & Bắt đầu](01-cai-dat.md) | Cài đặt, tạo bot Telegram, cấu hình lần đầu |
| 2 | [Cấu hình](02-cau-hinh.md) | File cấu hình, biến môi trường, các tuỳ chọn |
| 3 | [Kubernetes](03-kubernetes.md) | Quản lý pod, node, deployment, logs, metrics |
| 4 | [ArgoCD & GitOps](04-argocd.md) | Sync, rollback, dashboard, deployment freeze |
| 5 | [Helm](05-helm.md) | Xem và rollback Helm release |
| 6 | [Giám sát & Cảnh báo](06-watcher.md) | Real-time alerts: OOM, node down, cert hết hạn, PVC |
| 7 | [Incident & Briefing](07-incident-briefing.md) | Timeline sự cố, báo cáo sức khoẻ hằng ngày |
| 8 | [RBAC & Phân quyền](08-rbac.md) | Vai trò người dùng, phân quyền, approval workflow |
| 9 | [Audit Log](09-audit.md) | Xem nhật ký hành động |
| 10 | [Tham chiếu lệnh](10-lenh.md) | Danh sách đầy đủ tất cả lệnh |

---

## Giới thiệu nhanh

Telekube biến Telegram thành một Command Center mạnh mẽ để:

- **Quản lý Kubernetes** — xem pod, log, sự kiện, scale, restart, node management
- **GitOps với ArgoCD** — sync app, rollback, xem diff, deployment freeze
- **Giám sát chủ động** — cảnh báo real-time khi pod crash, OOMKill, node down, cert hết hạn
- **Quản lý Helm** — list release, xem lịch sử, rollback nhanh
- **Phản ứng sự cố** — timeline sự cố tự động từ K8s events và audit log
- **Báo cáo hằng ngày** — cluster health summary lên Telegram mỗi sáng
- **Quản trị** — RBAC, approval workflow, audit log đầy đủ

---

_Cập nhật lần cuối: 2026-03-18_
