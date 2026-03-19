# Part 1 — Installation & Getting Started

> **Objective:** Install Telekube, create a Telegram Bot, connect a Kubernetes cluster, and start the bot for the first time.

---

## 1.1 System Requirements

| Component | Minimum Version | Notes |
|-----------|-----------------|-------|
| Go | 1.25+ | Required only if building from source |
| Docker | 20.10+ | Optional, if running as a container |
| Kubernetes | 1.25+ | Requires access to the cluster |
| Telegram Bot | — | Token from @BotFather |

---

## 1.2 Create a Telegram Bot

Before installing Telekube, you need to create a Telegram Bot:

1. Open Telegram and search for **[@BotFather](https://t.me/BotFather)**
2. Send the `/newbot` command
3. Enter a display name for the bot (e.g., `My Kube Bot`)
4. Enter a username for the bot, must end in `bot` (e.g., `mykube_bot`)
5. BotFather will return a **token** that looks like this:
   ```
   123456789:ABCdefGHIjklMNOpqrSTUvwxYZ
   ```
6. **Save this token** — this is your `TELEKUBE_TELEGRAM_TOKEN`

**Get your Telegram User ID:**
1. Search for **[@userinfobot](https://t.me/userinfobot)** on Telegram
2. Send any message and the bot will reply with your ID
3. Save this ID — this is your `TELEKUBE_TELEGRAM_ADMIN_IDS`

---

## 1.3 Installation

### Option 1: Interactive Setup Wizard (Recommended)

This is the fastest method for new users:

```bash
# Clone the repository
git clone https://github.com/d9042n/telekube.git
cd telekube

# Build binary
make build

# Run interactive setup wizard
./bin/telekube setup
```

The wizard will prompt step-by-step:
- Telegram Bot Token
- Admin User IDs
- Kubeconfig path
- Storage backend (SQLite or PostgreSQL)
- Modules to enable

Once complete, the wizard creates the `configs/config.yaml` file. Start the bot:

```bash
./bin/telekube serve --config configs/config.yaml
```

---

### Option 2: Manual Configuration

```bash
# Clone and build
git clone https://github.com/d9042n/telekube.git
cd telekube
make build

# Copy example config file
cp configs/config.example.yaml configs/config.yaml

# Edit config file
nano configs/config.yaml
```

Edit at least the following fields in `config.yaml`:

```yaml
telegram:
  token: "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"   # Token from BotFather
  admin_ids: [123456789]                            # Your User ID

clusters:
  - name: "my-cluster"
    display_name: "Production"
    kubeconfig: "/home/user/.kube/config"           # Kubeconfig path
    default: true
```

Start the bot:

```bash
make run
# or
./bin/telekube serve --config configs/config.yaml
```

---

### Option 3: Docker

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

> **Note:** The `/data` volume is used to store the SQLite database. If not mounted, RBAC data and audit logs will be lost when the container restarts.

---

### Option 4: Helm on Kubernetes

Install Telekube like an app inside the cluster you want to manage:

```bash
# Add Helm repo
helm install telekube deploy/helm/telekube \
  --namespace telekube \
  --create-namespace \
  --set config.telegram.token="$TELEKUBE_TELEGRAM_TOKEN" \
  --set config.telegram.adminIDs="{123456789}" \
  --set config.storage.backend=postgres \
  --set config.storage.postgres.dsn="postgres://user:pass@postgres:5432/telekube"
```

When running inside the cluster, Telekube automatically uses **in-cluster config** — no need to mount kubeconfig.

---

## 1.4 First Start

1. Once the bot is running, open Telegram and search for your bot by username
2. Send the `/start` command — you will see a welcome screen and the cluster list
3. Send `/help` to see all available commands
4. Try `/pods` to view the list of pods in the cluster

### Checking if the bot is running

Telekube has health check endpoints:

```bash
# Defaults to port 8080
curl http://localhost:8080/healthz
# {"status":"ok"}

curl http://localhost:8080/readyz
# {"status":"ok","checks":{"kubernetes":"ok","storage":"ok"}}
```

---

## 1.5 View Logs

```bash
# If running directly
./bin/telekube serve --config configs/config.yaml 2>&1 | tee telekube.log

# If running Docker
docker logs -f telekube

# If running Helm/Kubernetes
kubectl logs -n telekube deployment/telekube -f
```

---

## Next Steps

- [Detailed Configuration →](02-configuration.md)
- [Kubernetes Guide →](03-kubernetes.md)
- [User Authorization (RBAC) →](08-rbac.md)
