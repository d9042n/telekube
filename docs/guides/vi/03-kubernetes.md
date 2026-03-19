# Phần 3 — Kubernetes

> **Mục tiêu:** Quản lý pod, node, deployment, xem logs, metrics, và thực hiện các thao tác vận hành trực tiếp từ Telegram.

---

## 3.1 Yêu cầu

Module Kubernetes bật mặc định. Trong `config.yaml`:

```yaml
modules:
  kubernetes:
    enabled: true
```

---

## 3.2 Pods — Quản lý Pod

### `/pods` — Danh sách pod

```
/pods [namespace]
```

**Cách sử dụng:**

1. Gửi `/pods` — bot hiển thị danh sách namespace dưới dạng inline button
2. Nhấn chọn namespace (ví dụ: `production`)
3. Bot liệt kê các pod với trạng thái:
   - `Running` — đang hoạt động bình thường
   - `Pending` — đang khởi động
   - `CrashLoopBackOff` — đang crash liên tục
   - `OOMKilled` — bị kill do hết memory
4. Nhấn vào tên pod để xem chi tiết

**Hoặc chỉ định namespace trực tiếp:**

```
/pods production
```

**Thông tin hiển thị mỗi pod:**
- Tên pod và trạng thái
- Số lần restart
- Thời gian chạy (age)
- IP của pod

**Phân quyền tối thiểu:** `viewer`

---

### `/logs <tên-pod>` — Xem logs

```
/logs my-app-7d8f9b-xvz4k
```

**Cách sử dụng:**

1. Gửi `/logs tên-pod` hoặc nhấn nút **Logs** từ màn hình Pod Detail
2. Nếu pod có nhiều container, bot hiển thị danh sách container để chọn
3. Bot trả về 100 dòng log cuối cùng
4. Nhấn **Xem thêm** (`Load More`) để tải thêm log cũ hơn

**Tính năng:**
- Xem log theo từng container
- Phân trang để xem log cũ
- Mã màu ERROR/WARN được giữ nguyên trong định dạng monospace

**Phân quyền tối thiểu:** `viewer`

---

### `/events <tên-pod>` — Xem sự kiện

```
/events my-app-7d8f9b-xvz4k
```

Hiển thị Kubernetes Events liên quan đến pod: Scheduled, Pulled, Started, BackOff, OOMKilling...

**Phân quyền tối thiểu:** `viewer`

---

### Restart Pod

1. Nhấn vào pod → màn hình Pod Detail
2. Nhấn nút **Restart**
3. Bot hỏi xác nhận: nhấn **Xác nhận** để tiến hành
4. Bot thực hiện xoá pod (K8s sẽ tự tạo lại)

> **Cơ chế:** Restart pod = xoá pod. Deployment/StatefulSet sẽ tự tạo pod mới.

**Phân quyền tối thiểu:** `operator`

---

## 3.3 `/top` — Metrics CPU & RAM

```
/top
/top nodes
```

### Xem metrics pod

1. Gửi `/top` — chọn namespace
2. Bot hiển thị bảng metrics:
   - Pod name
   - CPU usage (ví dụ: `245m` = 245 millicores)
   - Memory usage (ví dụ: `512Mi`)
3. Nhấn **Refresh** để cập nhật
4. Nhấn **Nodes** để chuyển sang xem node metrics

### Xem metrics node

1. Gửi `/top nodes` trực tiếp
2. Bot liệt kê tất cả node với CPU%, RAM%
3. Hiển thị node nào đang quá tải

> **Yêu cầu:** Cluster phải cài **metrics-server**. Nếu không có, bot thông báo lỗi.

**Phân quyền tối thiểu:** `viewer`

---

## 3.4 `/scale` — Scale Deployment

```
/scale
```

**Cách sử dụng:**

1. Gửi `/scale` — chọn namespace
2. Chọn Deployment hoặc StatefulSet muốn scale
3. Bot hiển thị số replica hiện tại
4. Chọn số replica mới từ các nút: **1**, **2**, **3**, **5**, **10**, hoặc nhập tay
5. Xác nhận thao tác
6. Bot cập nhật replica count và thông báo kết quả

**Phân quyền tối thiểu:** `operator`

> **Lưu ý:** Nếu bật approval workflow, thao tác scale trên cluster production có thể yêu cầu được duyệt bởi admin.

---

## 3.5 `/nodes` — Quản lý Node

```
/nodes
```

### Xem danh sách node

Bot hiển thị danh sách node với:
- Tên node
- Trạng thái: `Ready` / `NotReady` / `SchedulingDisabled`
- Role: `control-plane` / `worker`
- Thời gian hoạt động

Nhấn vào tên node để xem chi tiết:
- Thông tin phần cứng (CPU, RAM)
- Tình trạng disk
- Pods đang chạy trên node
- Labels và taints

### Cordon Node (Dừng lên lịch)

> Ngăn không cho pod mới được lên lịch vào node này (pod hiện tại không bị ảnh hưởng).

1. Nhấn vào node → **Cordon**
2. Xác nhận
3. Node chuyển sang trạng thái `SchedulingDisabled`

**Phân quyền tối thiểu:** `admin`

### Uncordon Node (Mở lại)

1. Nhấn vào node đã ở trạng thái `SchedulingDisabled` → **Uncordon**
2. Xác nhận
3. Node trở lại trạng thái `Ready`

**Phân quyền tối thiểu:** `admin`

### Drain Node (Di tản workload)

> Cordon + xoá tất cả pod trên node → node trống để bảo trì.

1. Nhấn vào node → **Drain**
2. Bot cảnh báo: tất cả pod sẽ bị di chuyển
3. Xác nhận kỹ lưỡng
4. Bot thực hiện drain (timeout 5 phút)

**Phân quyền tối thiểu:** `admin`

> **Cảnh báo:** Drain là thao tác ảnh hưởng lớn. Đảm bảo workload có đủ replica trước khi drain.

---

## 3.6 `/quota` — Resource Quota

```
/quota
```

**Cách sử dụng:**

1. Gửi `/quota` — chọn namespace
2. Bot hiển thị Resource Quota của namespace:
   - CPU limit / request
   - Memory limit / request  
   - Số pod tối đa
   - Phần trăm đã sử dụng (thanh progress)

**Phân quyền tối thiểu:** `viewer`

---

## 3.7 `/namespaces` — Danh sách Namespace

```
/namespaces
```

Liệt kê tất cả namespace trong cluster với trạng thái (`Active` / `Terminating`).

**Phân quyền tối thiểu:** `viewer`

---

## 3.8 `/restart <pod>` — Restart Pod

```
/restart <tên-pod> [namespace]
```

Restart pod bằng cách xoá pod (controller sẽ tự tạo lại). Chỉ hoạt động với pod được quản lý bởi Deployment, StatefulSet, hoặc DaemonSet.

- Hiển thị hộp thoại xác nhận trước khi thực hiện
- Không thể restart pod standalone (không có controller)
- Hành động được ghi vào audit trail

**Phân quyền tối thiểu:** `operator`

---

## 3.9 `/deploys` — Danh sách Deployment

```
/deploys [namespace]
```

Liệt kê Deployment theo namespace với trạng thái:
- Số replica mong muốn vs đang chạy
- `Available` / `Progressing` / `Degraded`

**Phân quyền tối thiểu:** `viewer`

---

## 3.10 `/cronjobs` — Trạng thái CronJob

```
/cronjobs [namespace]
```

Liệt kê CronJob với:
- Schedule (cron expression)
- Last run time
- Trạng thái lần chạy cuối (Success/Failed)
- Số lần chạy failed gần đây

**Phân quyền tối thiểu:** `viewer`

---

## 3.11 Chuyển đổi Cluster

```
/clusters
```

Hiển thị danh sách cluster đã cấu hình. Nhấn chọn cluster để chuyển sang làm việc với cluster đó.

Mọi lệnh sau đó (`/pods`, `/nodes`, v.v.) sẽ hoạt động trên cluster đã chọn.

**Phân quyền tối thiểu:** `viewer` (tất cả)

---

## 3.12 Bảng tóm tắt lệnh Kubernetes

| Lệnh | Mô tả | Phân quyền |
|------|-------|-----------|
| `/pods [ns]` | Danh sách pod | viewer |
| `/logs <pod>` | Xem log pod | viewer |
| `/events <pod>` | Xem events pod | viewer |
| `/top` | Metrics CPU/RAM của pod | viewer |
| `/top nodes` | Metrics CPU/RAM của node | viewer |
| `/scale` | Scale deployment/statefulset | operator |
| `/restart <pod>` | Restart pod | operator |
| `/nodes` | Danh sách và quản lý node | viewer |
| `/quota` | Resource quota namespace | viewer |
| `/namespaces` | Danh sách namespace | viewer |
| `/deploys` | Danh sách deployment | viewer |
| `/cronjobs` | Trạng thái cronjob | viewer |
| `/clusters` | Chuyển cluster | viewer |

---

## Bước tiếp theo

- [ArgoCD & GitOps →](04-argocd.md)
- [Helm Management →](05-helm.md)
- [Giám sát & Cảnh báo →](06-watcher.md)
