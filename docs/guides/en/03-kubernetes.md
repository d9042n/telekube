# Part 3 — Kubernetes

> **Objective:** Manage pods, nodes, deployments, view logs, metrics, and perform direct operations via Telegram.

---

## 3.1 Requirements

The Kubernetes module is enabled by default in `config.yaml`:

```yaml
modules:
  kubernetes:
    enabled: true
```

---

## 3.2 Pods — Pod Management

### `/pods` — Pod List

```
/pods [namespace]
```

**How to use:**

1. Send `/pods` — the bot displays the list of namespaces as inline buttons
2. Click on a namespace (e.g., `production`)
3. The bot lists the pods with their statuses:
   - `Running` — operating normally
   - `Pending` — starting up
   - `CrashLoopBackOff` — crashing continuously
   - `OOMKilled` — killed due to Out Of Memory
4. Click on a pod name to view its details

**Or specify the namespace directly:**

```
/pods production
```

**Information displayed per pod:**
- Pod name and status
- Restart count
- Uptime (age)
- Pod IP

**Minimum role:** `viewer`

---

### `/logs <pod-name>` — View Logs

```
/logs my-app-7d8f9b-xvz4k
```

**How to use:**

1. Send `/logs <pod-name>` or click the **Logs** button from the Pod Detail screen
2. If the pod has multiple containers, the bot asks you to select a container
3. The bot returns the last 100 log lines
4. Click **Load More** to fetch older logs

**Features:**
- View logs for each container
- Pagination to see old logs
- ERROR/WARN color codes are retained in monospace output

**Minimum role:** `viewer`

---

### `/events <pod-name>` — View Events

```
/events my-app-7d8f9b-xvz4k
```

Displays the Kubernetes Events associated with a pod: Scheduled, Pulled, Started, BackOff, OOMKilling...

**Minimum role:** `viewer`

---

### Restart Pod

1. Click on a pod → Pod Detail screen
2. Click the **Restart** button
3. The bot prompts for confirmation: click **Confirm** to proceed
4. The bot deletes the pod (K8s will recreate it automatically)

> **Mechanism:** Restart pod = delete pod. The Deployment/StatefulSet will spawn a new pod.

**Minimum role:** `operator`

---

## 3.3 `/top` — CPU & RAM Metrics

```
/top
/top nodes
```

### View Pod Metrics

1. Send `/top` — select a namespace
2. The bot displays a metrics table:
   - Pod name
   - CPU usage (e.g., `245m` = 245 millicores)
   - Memory usage (e.g., `512Mi`)
3. Click **Refresh** to update
4. Click **Nodes** to switch to the node metrics view

### View Node Metrics

1. Send `/top nodes` directly
2. The bot lists all nodes with CPU%, RAM%
3. Displays any overloaded nodes

> **Requirement:** The cluster must have the **metrics-server** installed. The bot returns an error otherwise.

**Minimum role:** `viewer`

---

## 3.4 `/scale` — Scale Deployment

```
/scale
```

**How to use:**

1. Send `/scale` — select a namespace
2. Choose the Deployment or StatefulSet to scale
3. The bot displays the current replica count
4. Select the new replica count from the grid: **1**, **2**, **3**, **5**, **10**, or type manually
5. Confirm the action
6. The bot updates the replica count and announces the result

**Minimum role:** `operator`

> **Note:** If the approval workflow is enabled, scaling on production clusters might require admin approval.

---

## 3.5 `/nodes` — Node Management

```
/nodes
```

### View Node List

The bot returns a list of nodes showing:
- Node name
- Status: `Ready` / `NotReady` / `SchedulingDisabled`
- Role: `control-plane` / `worker`
- Uptime

Click a node's name to view details:
- Hardware profile (CPU, RAM)
- Disk conditions
- Running pods on the node
- Labels and taints

### Cordon Node (Disable Scheduling)

> Prevents new pods from being scheduled onto this node (existing pods are unaffected).

1. Click on a node → **Cordon**
2. Confirm
3. The node transition to `SchedulingDisabled` status

**Minimum role:** `admin`

### Uncordon Node (Re-enable)

1. Click on a `SchedulingDisabled` node → **Uncordon**
2. Confirm
3. The node reverts back to `Ready` status

**Minimum role:** `admin`

### Drain Node (Evict Workloads)

> Cordon + delete all pods on the node → emptying the node for maintenance.

1. Click on a node → **Drain**
2. The bot issues a warning: all pods will be migrated
3. Carefully confirm
4. The bot performs the drain action (5-minute timeout)

**Minimum role:** `admin`

> **Warning:** Eviction is heavy. Ensure your workload has adequate replicas before draining.

---

## 3.6 `/quota` — Resource Quotas

```
/quota
```

**How to use:**

1. Send `/quota` — select a namespace
2. The bot returns the Resource Quotas for that namespace:
   - CPU limit / requests
   - Memory limit / requests  
   - Max pod count
   - Used percentage (progress bar)

**Minimum role:** `viewer`

---

## 3.7 `/namespaces` — Namespace List

```
/namespaces
```

Lists all namespaces within the cluster along with their status (`Active` / `Terminating`).

**Minimum role:** `viewer`

---

## 3.8 `/restart <pod>` — Restart Pod

```
/restart <pod-name> [namespace]
```

Restarts a pod by deleting it (the controller will recreate it). Only works for pods managed by a Deployment, StatefulSet, or DaemonSet.

- Confirmation dialog is shown before executing
- Standalone pods (without a controller) cannot be restarted
- Action is logged to the audit trail

**Minimum role:** `operator`

---

## 3.9 `/deploys` — Deployment List

```
/deploys [namespace]
```

Returns Deployments by namespace with their status showing:
- Desired vs Running replicas
- `Available` / `Progressing` / `Degraded`

**Minimum role:** `viewer`

---

## 3.10 `/cronjobs` — CronJob Status

```
/cronjobs [namespace]
```

Lists CronJobs providing:
- Schedule (cron expression)
- Last run time
- Last run status (Success/Failed)
- Recent failed run count

**Minimum role:** `viewer`

---

## 3.11 Switch Clusters

```
/clusters
```

Shows the list of configured clusters. Click a cluster to switch and act upon that cluster.

Subsequent commands (`/pods`, `/nodes`, etc.) will execute on the selected cluster.

**Minimum role:** `viewer` (all users)

---

## 3.12 Kubernetes Command Summary

| Command | Description | Role |
|---------|-------------|------|
| `/pods [ns]` | Pod list | viewer |
| `/logs <pod>` | Read pod logs | viewer |
| `/events <pod>` | Read pod events | viewer |
| `/top` | Pod CPU/RAM metrics | viewer |
| `/top nodes` | Node CPU/RAM metrics | viewer |
| `/scale` | Scale deployment/statefulset | operator |
| `/restart <pod>` | Restart a pod | operator |
| `/nodes` | List and manage nodes | viewer |
| `/quota` | Namespace resource quotas | viewer |
| `/namespaces` | Namespace list | viewer |
| `/deploys` | Deployment list | viewer |
| `/cronjobs` | CronJob status | viewer |
| `/clusters` | Switch cluster | viewer |

---

## Next Steps

- [ArgoCD & GitOps →](04-argocd.md)
- [Helm Management →](05-helm.md)
- [Monitoring & Alerting →](06-watcher.md)
