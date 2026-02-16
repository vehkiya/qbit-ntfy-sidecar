# qBit-Ntfy Sidecar

A lightweight Go sidecar for Kubernetes to monitor qBittorrent downloads and send real-time progress updates to [ntfy.sh](https://ntfy.sh) or a self-hosted ntfy server.

## Features
- **Event-Driven**: Only runs when triggered by qBittorrent (zero idle CPU usage).
- **Startup Sync**: Automatically resumes monitoring active downloads on container restart.
- **Real-time Progress**: Sends updates with ASCII progress bars or percentages.
- **Completion Alerts**: Configurable priority notification when download finishes.
- **Flexible Auth**: Supports both authenticated and localhost-bypass access to qBittorrent.

## Installation

### Option A: Kubernetes (Recommended)
Add the sidecar container to your qBittorrent Deployment.

```yaml
      containers:
        - name: qbittorrent
          image: lscr.io/linuxserver/qbittorrent:latest
          # ... your existing qbit config ...

        - name: ntfy-sidecar
          image: ghcr.io/vehkiya/qbit-ntfy-sidecar:latest
          imagePullPolicy: Always
          resources:
            requests:
              cpu: "10m"
              memory: "32Mi"
            limits:
              cpu: "100m"
              memory: "128Mi"
          env:
            - name: NTFY_TOPIC
              value: "my_downloads"
            - name: NTFY_SERVER
              value: "https://ntfy.sh"
            # Optional: If you need Ntfy Auth
            # - name: NTFY_USER
            #   valueFrom: { secretKeyRef: { name: ntfy-secrets, key: username } }
            # - name: NTFY_PASS
            #   valueFrom: { secretKeyRef: { name: ntfy-secrets, key: password } }
```

### Option B: Docker Compose
Add the sidecar service to your `docker-compose.yml`. It needs to share the network with qBittorrent to access the API at `localhost`.

```yaml
services:
  qbittorrent:
    image: lscr.io/linuxserver/qbittorrent:latest
    network_mode: service:sidecar # or share a bridge network
    # ...

  sidecar:
    image: ghcr.io/vehkiya/qbit-ntfy-sidecar:latest
    container_name: qbit-ntfy-sidecar
    environment:
      - QBIT_HOST=http://qbittorrent:8080 # Use container name if not sharing network stack
      - NTFY_TOPIC=my_downloads
      - NTFY_SERVER=https://ntfy.sh
    restart: unless-stopped
```

### Option C: Standalone Docker
Make sure the sidecar can reach the qBittorrent container (e.g., share a network).
```bash
docker run -d \
  --name qbit-ntfy-sidecar \
  --network=container:qbittorrent \
  -e NTFY_TOPIC=my_downloads \
  ghcr.io/vehkiya/qbit-ntfy-sidecar:latest
```

## Configuration Steps

### 1. Configure qBittorrent Auth
**Option A (Easiest): Bypass Localhost Auth**
1. Open qBittorrent Web UI (`Tools > Options > Web UI`).
2. Under **Authentication**, check **"Bypass authentication for clients on localhost"**.
3. This works perfectly if the sidecar is in the same Pod (Kubernetes) or sharing the network stack (Docker).

**Option B: Explicit Auth**
If you cannot bypass auth, set the following env vars in the sidecar:
- `QBIT_USER=admin`
- `QBIT_PASS=your_password`

### 2. Setup "Run on Added" Trigger
The sidecar is event-driven. It needs to know *when* to start monitoring a new torrent.

1. Open qBittorrent Web UI.
2. Go to `Tools > Options > Downloads`.
3. Check **"Run external program on torrent added"**.
4. Enter the trigger command:
   ```bash
   curl -X POST "http://localhost:9090/track?hash=%I"
   ```
   *(Note: If using Docker Compose/K8s with separate IPs, replace `localhost` with the sidecar's hostname/IP).*

> **Note on Completion**: You do **not** need to configure "Run external program on torrent finished". The sidecar automatically detects completion.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `QBIT_HOST` | qBittorrent API URL | `http://localhost:8080` |
| `QBIT_USER` | Web UI Username (Optional) | `""` |
| `QBIT_PASS` | Web UI Password (Optional) | `""` |
| `NTFY_TOPIC` | Ntfy Topic Name (**REQUIRED**) | `null` |
| `NTFY_SERVER` | Ntfy Server URL | `https://ntfy.sh` |
| `NTFY_USER` | Ntfy Username (Optional) | `""` |
| `NTFY_PASS` | Ntfy Password (Optional) | `""` |
| `NTFY_PRIORITY_PROGRESS` | Priority for progress updates | `2` (Low) |
| `NTFY_PRIORITY_COMPLETE` | Priority for completion alerts | `3` (Default) |
| `NOTIFY_COMPLETE` | Send notification on completion | `true` |
| `PROGRESS_FORMAT` | Format: `bar` or `percent` | `bar` |
| `POLL_INTERVAL` | Polling interval in seconds | `5` |

## Building Locally
```bash
go build -o sidecar main.go
```

## Docker Build
```bash
docker build -t qbit-ntfy-sidecar .
```
