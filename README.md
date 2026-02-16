# qBit-Ntfy Sidecar

A lightweight Go sidecar for Kubernetes to monitor qBittorrent downloads and send real-time progress updates to [ntfy.sh](https://ntfy.sh) or a self-hosted ntfy server.

## Features
- **Event-Driven**: Only runs when triggered by qBittorrent (zero idle CPU usage).
- **Startup Sync**: Automatically resumes monitoring active downloads on container restart.
- **Real-time Progress**: Sends updates with ASCII progress bars or percentages.
- **Completion Alerts**: Configurable priority notification when download finishes.
- **Flexible Auth**: Supports both authenticated and localhost-bypass access to qBittorrent.

## Installation

### 1. Deploy Sidecar
Add the sidecar container to your qBittorrent deployment.

```yaml
- name: ntfy-sidecar
  image: ghcr.io/vehkiya/qbit-ntfy-sidecar:latest
  env:
    - name: NTFY_TOPIC
      value: "my_downloads"
    # Optional: Self-hosted server (default: https://ntfy.sh)
    - name: NTFY_SERVER
      value: "https://ntfy.example.com"
```

### 2. Configure qBittorrent

#### A. Enable Access (Recommended)
To allow the sidecar to access the API without storing credentials:
1. Open qBittorrent Web UI (`Tools > Options > Web UI`).
2. Under **Authentication**, check **"Bypass authentication for clients on localhost"**.
3. *Alternatively, if you cannot enable this, set `QBIT_USER` and `QBIT_PASS` env vars in the sidecar.*

#### B. Setup Triggers
1. Navigate to (`Tools > Options > Downloads`).
2. Check **"Run external program on torrent added"**.
3. Enter the trigger command:
   ```bash
   curl -X POST "http://localhost:9090/track?hash=%I"
   ```

> **Note on Completion**: The sidecar automatically detects when a download finishes (via polling) and sends the completion notification. You do **not** need to configure "Run external program on torrent finished".

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

## Building Locally
```bash
go build -o sidecar main.go
```

## Docker Build
```bash
docker build -t qbit-ntfy-sidecar .
```
