# Phần 1 — Cài Đặt & Bắt Đầu

> **Mục tiêu:** Cài đặt Telekube, tạo Telegram Bot, kết nối cluster Kubernetes và khởi động bot lần đầu.

---

## 1.1 Yêu cầu hệ thống

| Thành phần | Phiên bản tối thiểu | Ghi chú |
|-----------|---------------------|---------|
| Go | 1.25+ | Chỉ cần nếu build từ source |
| Docker | 20.10+ | Tuỳ chọn, nếu chạy container |
| Kubernetes | 1.25+ | Cần quyền truy cập vào cluster |
| Telegram Bot | — | Token từ @BotFather |

---

## 1.2 Tạo Telegram Bot

Trước khi cài đặt Telekube, bạn cần tạo một Telegram Bot:

1. Mở Telegram và tìm kiếm **[@BotFather](https://t.me/BotFather)**
2. Gửi lệnh `/newbot`
3. Nhập tên hiển thị cho bot (ví dụ: `My Kube Bot`)
4. Nhập username cho bot, phải kết thúc bằng `bot` (ví dụ: `mykube_bot`)
5. BotFather sẽ trả về một **token** có dạng:
   ```
   123456789:ABCdefGHIjklMNOpqrSTUvwxYZ
   ```
6. **Lưu token này lại** — đây là `TELEKUBE_TELEGRAM_TOKEN`

**Lấy Telegram User ID của bạn:**
1. Tìm kiếm **[@userinfobot](https://t.me/userinfobot)** trên Telegram
2. Gửi bất kỳ tin nhắn nào và bot sẽ trả về ID của bạn
3. Lưu ID này — đây là `TELEKUBE_TELEGRAM_ADMIN_IDS`

---

## 1.3 Cài đặt

### Phương án 1: Wizard Cài đặt Tương tác (Khuyến nghị)

Đây là cách nhanh nhất cho người dùng mới:

```bash
# Clone repository
git clone https://github.com/d9042n/telekube.git
cd telekube

# Build binary
make build

# Chạy wizard cài đặt tương tác
./bin/telekube setup
```

Wizard sẽ hỏi từng bước:
- Telegram Bot Token
- Admin User IDs
- Kubeconfig path
- Storage backend (SQLite hoặc PostgreSQL)
- Các module muốn bật

Sau khi hoàn tất, wizard tạo file `configs/config.yaml`. Khởi động bot:

```bash
./bin/telekube serve --config configs/config.yaml
```

---

### Phương án 2: Cấu hình thủ công

```bash
# Clone và build
git clone https://github.com/d9042n/telekube.git
cd telekube
make build

# Sao chép file cấu hình mẫu
cp configs/config.example.yaml configs/config.yaml

# Chỉnh sửa file cấu hình
nano configs/config.yaml
```

Chỉnh sửa ít nhất các trường sau trong `config.yaml`:

```yaml
telegram:
  token: "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"   # Token từ BotFather
  admin_ids: [123456789]                            # User ID của bạn

clusters:
  - name: "my-cluster"
    display_name: "Production"
    kubeconfig: "/home/user/.kube/config"           # Đường dẫn kubeconfig
    default: true
```

Khởi động bot:

```bash
make run
# hoặc
./bin/telekube serve --config configs/config.yaml
```

---

### Phương án 3: Docker

```bash
docker run -d \
  --name telekube \
  --restart unless-stopped \
  -e TELEKUBE_TELEGRAM_TOKEN="123456789:ABCdefGHIjklMNOpqrSTUvwxYZ" \
  -e TELEKUBE_TELEGRAM_ADMIN_IDS="123456789" \
  -v ~/.kube/config:/root/.kube/config:ro \
  -v $(pwd)/data:/data \
  ghcr.io/d9042n/telekube:latest
```

> **Lưu ý:** Volume `/data` dùng để lưu SQLite database. Nếu không mount, dữ liệu RBAC và audit log sẽ mất khi container restart.

---

### Phương án 4: Helm trên Kubernetes

Cài đặt Telekube như một ứng dụng trong chính cluster cần quản lý:

```bash
# Thêm Helm repo
helm install telekube deploy/helm/telekube \
  --namespace telekube \
  --create-namespace \
  --set config.telegram.token="$TELEKUBE_TELEGRAM_TOKEN" \
  --set config.telegram.adminIDs="{123456789}" \
  --set config.storage.backend=postgres \
  --set config.storage.postgres.dsn="postgres://user:pass@postgres:5432/telekube"
```

Khi chạy trong cluster, Telekube tự động sử dụng **in-cluster config** — không cần mount kubeconfig.

---

## 1.4 Khởi động lần đầu

1. Sau khi bot đang chạy, mở Telegram và tìm bot của bạn theo username
2. Gửi lệnh `/start` — bạn sẽ thấy màn hình chào mừng và danh sách cluster
3. Gửi `/help` để xem tất cả lệnh có sẵn
4. Thử `/pods` để xem danh sách pod trong cluster

### Kiểm tra bot đang chạy

Telekube có endpoint health check:

```bash
# Mặc định ở port 8080
curl http://localhost:8080/healthz
# {"status":"ok"}

curl http://localhost:8080/readyz
# {"status":"ok","checks":{"kubernetes":"ok","storage":"ok"}}
```

---

## 1.5 Xem logs

```bash
# Nếu chạy trực tiếp
./bin/telekube serve --config configs/config.yaml 2>&1 | tee telekube.log

# Nếu chạy Docker
docker logs -f telekube

# Nếu chạy Helm/Kubernetes
kubectl logs -n telekube deployment/telekube -f
```

---

## Bước tiếp theo

- [Cấu hình chi tiết →](02-cau-hinh.md)
- [Hướng dẫn sử dụng Kubernetes →](03-kubernetes.md)
- [Phân quyền người dùng (RBAC) →](08-rbac.md)
