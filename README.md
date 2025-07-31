# SyncArr 🎬📺🔃

![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?style=for-the-badge&logo=docker&logoColor=white)
![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Plex](https://img.shields.io/badge/plex-%23E5A00D.svg?style=for-the-badge&logo=plex&logoColor=white)

**SyncArr** is a high-performance Go application that synchronizes labeled movies and TV shows between Plex Media Servers. It provides fast file transfers using rsync, comprehensive metadata synchronization, and intelligent content matching to keep your Plex libraries perfectly synchronized.

## 🚀 Quick Start

### Docker Compose Example

```yaml
version: '3.8'

services:
  syncarr:
    image: syncarr:latest
    container_name: syncarr
    restart: unless-stopped
    
    environment:
      # Source Plex Server
      SOURCE_PLEX_REQUIRES_HTTPS: "true"
      SOURCE_PLEX_HOST: "192.168.1.10"
      SOURCE_PLEX_PORT: "32400"
      SOURCE_PLEX_TOKEN: "your-source-plex-token"
      
      # Destination Plex Server
      DEST_PLEX_REQUIRES_HTTPS: "true"
      DEST_PLEX_HOST: "192.168.1.20"
      DEST_PLEX_PORT: "32400"
      DEST_PLEX_TOKEN: "your-destination-plex-token"
      
      # SSH Configuration (choose password OR key-based auth)
      OPT_SSH_USER: "your-ssh-user"
      OPT_SSH_PASSWORD: "your-ssh-password"  # For password auth
      # OPT_SSH_KEY_PATH: "/keys/id_rsa"     # For key-based auth
      OPT_SSH_PORT: "22"
      
      # Sync Configuration
      SYNC_LABEL: "Sync2Secondary"           # Label to identify content to sync
      SYNC_INTERVAL: "60"                    # Minutes between sync cycles
      LOG_LEVEL: "INFO"                      # DEBUG, INFO, WARN, ERROR
      DRY_RUN: "false"                       # Set to "true" for testing
      
      # Path Mapping
      SOURCE_REPLACE_FROM: "/data/Media"     # Source path prefix to replace
      SOURCE_REPLACE_TO: "/media/source"     # Local container path
      DEST_ROOT_DIR: "/mnt/data"             # Destination server root path
    
    volumes:
      # Mount your media directories (adjust paths as needed)
      - "/path/to/your/media:/media/source:ro"  # Read-only source media
      
      # For SSH key authentication (uncomment if using keys)
      # - "/path/to/ssh/keys:/keys:ro"
    
    # Use host networking to access local Plex servers
    network_mode: "host"
    
    # Health check
    healthcheck:
      test: ["CMD", "./syncarr", "--validate"]
      interval: 30s
      timeout: 10s
      retries: 3
```

> **💡 Pro Tip**: Start with `DRY_RUN: "true"` to test your configuration without making any changes!

## ✨ Features

<details>
<summary><strong>🎯 Core Synchronization Features</strong></summary>

- **🏷️ Label-based Sync**: Automatically sync only media items with specific Plex labels
- **⚡ High-Performance Transfers**: Uses rsync for fast, resumable file transfers
- **🔄 6-Phase Sync Process**: Content discovery → File transfer → Library refresh → Content matching → Metadata sync → Cleanup
- **📊 Comprehensive Metadata Sync**: Titles, summaries, ratings, genres, labels, collections, artwork, and more
- **👁️ Watched State Sync**: Keep viewing progress synchronized between servers
- **🔄 Incremental Updates**: Only transfer changed or new content
- **📁 Automatic Directory Creation**: Creates destination directories as needed

</details>

<details>
<summary><strong>🔐 Authentication & Security</strong></summary>

- **🔑 Dual SSH Authentication**: Support for both SSH keys and password authentication
- **🔒 Secure Transfers**: All file transfers use encrypted SSH connections
- **🛡️ Non-interactive Operation**: Uses sshpass for automated password authentication
- **⚠️ Dry Run Mode**: Test configurations without making any changes

</details>

<details>
<summary><strong>🛠️ Advanced Features</strong></summary>

- **🐳 Docker Ready**: Containerized application with health checks
- **📝 Structured Logging**: JSON logging with configurable levels (DEBUG, INFO, WARN, ERROR)
- **🔄 Continuous & One-shot Modes**: Run continuously or execute single sync cycles
- **🎛️ Force Full Sync**: Bypass incremental checks for complete re-synchronization
- **📈 Performance Monitoring**: Detailed transfer statistics and timing information
- **🔍 Content Matching**: Intelligent filename-based matching between source and destination

</details>

## 📋 Configuration

<details>
<summary><strong>🌍 Environment Variables</strong></summary>

### Plex Server Configuration

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `SOURCE_PLEX_HOST` | Source Plex server hostname/IP | `192.168.1.10` | ✅ |
| `SOURCE_PLEX_PORT` | Source Plex server port | `32400` | ❌ |
| `SOURCE_PLEX_TOKEN` | Source Plex server API token | `xxxxxxxxxxxx` | ✅ |
| `SOURCE_PLEX_REQUIRES_HTTPS` | Use HTTPS for source server | `true`/`false` | ❌ |
| `DEST_PLEX_HOST` | Destination Plex server hostname/IP | `192.168.1.20` | ✅ |
| `DEST_PLEX_PORT` | Destination Plex server port | `32400` | ❌ |
| `DEST_PLEX_TOKEN` | Destination Plex server API token | `xxxxxxxxxxxx` | ✅ |
| `DEST_PLEX_REQUIRES_HTTPS` | Use HTTPS for destination server | `true`/`false` | ❌ |

### SSH Configuration

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `OPT_SSH_USER` | SSH username | `mediauser` | ✅ |
| `OPT_SSH_PASSWORD` | SSH password (for password auth) | `secretpass` | ❌* |
| `OPT_SSH_KEY_PATH` | SSH private key path (for key auth) | `/keys/id_rsa` | ❌* |
| `OPT_SSH_PORT` | SSH port | `22` | ❌ |

*Either password or key path is required

### Sync Configuration

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `SYNC_LABEL` | Plex label to identify content to sync | `Sync2Secondary` | ✅ |
| `SYNC_INTERVAL` | Minutes between sync cycles | `60` | ❌ |
| `LOG_LEVEL` | Logging level | `INFO` | ❌ |
| `DRY_RUN` | Test mode without changes | `false` | ❌ |
| `FORCE_FULL_SYNC` | Force complete sync | `false` | ❌ |

### Path Mapping

| Variable | Description | Example | Required |
|----------|-------------|---------|----------|
| `SOURCE_REPLACE_FROM` | Source path prefix to replace | `/data/Media` | ❌ |
| `SOURCE_REPLACE_TO` | Container path for source media | `/media/source` | ❌ |
| `DEST_ROOT_DIR` | Destination server root directory | `/mnt/data` | ✅ |

</details>

<details>
<summary><strong>🎛️ Advanced Configuration</strong></summary>

### Performance Tuning

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKER_POOL_SIZE` | Number of concurrent workers | `4` |
| `PLEX_API_RATE_LIMIT` | Plex API requests per second | `10.0` |
| `TRANSFER_BUFFER_SIZE` | Transfer buffer size (KB) | `64` |
| `MAX_CONCURRENT_TRANSFERS` | Max simultaneous transfers | `3` |

### Transfer Options

| Variable | Description | Default |
|----------|-------------|---------|
| `ENABLE_COMPRESSION` | Enable transfer compression | `true` |
| `RESUME_TRANSFERS` | Resume interrupted transfers | `true` |

</details>

## 🚀 Usage

<details>
<summary><strong>📝 Getting Your Plex Token</strong></summary>

1. **Via Plex Web App:**
   - Open Plex Web App
   - Open browser developer tools (F12)
   - Go to Network tab
   - Refresh the page
   - Look for requests to `/library/sections`
   - Find the `X-Plex-Token` header value

2. **Via Plex API:**

   ```bash
   curl -X POST 'https://plex.tv/api/v2/users/signin' \
     -H 'Content-Type: application/x-www-form-urlencoded' \
     -d 'user[login]=YOUR_EMAIL&user[password]=YOUR_PASSWORD'
   ```

</details>

<details>
<summary><strong>🏷️ Adding Labels to Media</strong></summary>

1. **In Plex Web Interface:**
   - Navigate to your movie or TV show
   - Click "Edit" (pencil icon)
   - Go to "Tags" tab
   - Add your sync label (e.g., `Sync2Secondary`) to "Labels" field
   - Click "Save Changes"

2. **Bulk Labeling with Labelarr:**
   - Use [Labelarr](https://github.com/yourusername/labelarr) for bulk label management
   - Set up rules to automatically apply labels based on criteria

</details>

<details>
<summary><strong>🖥️ Command Line Usage</strong></summary>

```bash
# Run a single sync cycle
docker run --rm -v $(pwd)/config:/config syncarr --oneshot

# Validate configuration
docker run --rm -v $(pwd)/config:/config syncarr --validate

# Force full synchronization (bypasses incremental checks)
docker run --rm -v $(pwd)/config:/config syncarr --force-full-sync --oneshot

# Show version information
docker run --rm syncarr --version

# Run with debug logging
docker run --rm -e LOG_LEVEL=DEBUG syncarr --oneshot
```

</details>

<details>
<summary><strong>📊 Monitoring & Logs</strong></summary>

### View Logs

```bash
# Follow logs in real-time
docker-compose logs -f syncarr

# View last 100 lines
docker-compose logs --tail=100 syncarr

# Filter for errors only
docker-compose logs syncarr | grep '"level":"error"'
```

### Health Check

```bash
# Check container health
docker-compose ps

# Manual health check
docker-compose exec syncarr ./syncarr --validate
```

### Log Levels

- **DEBUG**: Detailed operation logs, file-by-file progress
- **INFO**: High-level status updates, sync summaries
- **WARN**: Non-critical issues, skipped items
- **ERROR**: Critical errors, failed operations

</details>

## 🏗️ Architecture

<details>
<summary><strong>📊 6-Phase Sync Process</strong></summary>

1. **🔍 Content Discovery**: Scan source Plex server for labeled media
2. **📂 File Transfer**: Copy media files using high-performance rsync
3. **🔄 Library Refresh**: Update destination Plex library
4. **🎯 Content Matching**: Match source items to destination items by filename
5. **📝 Metadata Sync**: Synchronize comprehensive metadata between matched items
6. **🧹 Cleanup**: Remove orphaned files and update statistics

</details>

<details>
<summary><strong>🧩 Components Overview</strong></summary>

```
┌─────────────────────┐    ┌─────────────────────┐
│   Source Plex       │    │  Destination Plex   │
│   Server            │    │  Server             │
└──────────┬──────────┘    └──────────┬──────────┘
           │                          │
           │          SyncArr         │
           │    ┌─────────────────┐   │
           └────┤ Sync Orchestrator├───┘
                └─────────┬───────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐    ┌───────▼────┐    ┌──────▼──────┐
   │Content  │    │File Transfer│    │Metadata     │
   │Discovery│    │  (rsync)    │    │Synchronizer │
   └─────────┘    └─────────────┘    └─────────────┘
```

**Key Components:**

- **🎯 Sync Orchestrator**: Coordinates the entire synchronization process
- **🔍 Content Discovery**: Finds labeled media using Plex API
- **📁 File Transfer**: High-performance rsync with automatic directory creation
- **📝 Metadata Synchronizer**: Comprehensive metadata and watched state sync
- **🔌 Plex Client**: Direct Plex API interactions with custom implementation
- **⚙️ Configuration Manager**: Environment-based configuration management

</details>

## 🔧 Development

<details>
<summary><strong>🏗️ Building from Source</strong></summary>

```bash
# Clone the repository
git clone https://github.com/nullable-eth/syncarr.git
cd syncarr

# Build the application
go build -o syncarr ./cmd/syncarr

# Run tests
go test ./...

# Build Docker image
docker build -t syncarr:latest .

# Run with development settings
LOG_LEVEL=DEBUG DRY_RUN=true ./syncarr --oneshot
```

</details>

<details>
<summary><strong>📁 Project Structure</strong></summary>

```
syncarr/
├── cmd/syncarr/              # Main application entry point
├── internal/
│   ├── config/               # Configuration management
│   ├── discovery/            # Content discovery and matching
│   ├── logger/               # Structured logging
│   ├── metadata/             # Metadata synchronization
│   ├── orchestrator/         # Main sync coordination
│   ├── plex/                 # Plex API client wrapper
│   └── transfer/             # File transfer (rsync/scp)
├── pkg/types/                # Shared data types
├── docker/                   # Docker configurations
├── scripts/                  # Utility scripts
├── Dockerfile                # Docker build configuration
└── README.md                 # This file
```

</details>

<details>
<summary><strong>🤝 Contributing</strong></summary>

We welcome contributions! Here's how to get started:

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/amazing-feature`
3. **Make your changes** and add tests
4. **Run tests**: `go test ./...`
5. **Build and test**: `docker build -t syncarr:test .`
6. **Commit changes**: `git commit -m 'Add amazing feature'`
7. **Push to branch**: `git push origin feature/amazing-feature`
8. **Open a Pull Request**

**Development Guidelines:**

- Follow Go best practices and `gofmt` formatting
- Add tests for new functionality
- Update documentation for user-facing changes
- Use structured logging with appropriate levels

</details>

## 🆘 Troubleshooting

<details>
<summary><strong>🔧 Common Issues</strong></summary>

### SSH Authentication Failed

```json
{"level":"error","msg":"Permission denied (publickey,password)"}
```

**Solutions:**

- Verify SSH credentials are correct
- Ensure SSH user has access to destination paths
- Test SSH connection manually: `ssh user@destination-server`
- For password auth: Ensure `OPT_SSH_PASSWORD` is set
- For key auth: Ensure private key is mounted and `OPT_SSH_KEY_PATH` is correct

### Rsync Not Found

```json
{"level":"error","msg":"rsync: command not found"}
```

**Solutions:**

- The Docker image includes rsync by default
- If building custom image, ensure rsync is installed
- Check container logs for rsync availability

### Directory Creation Failed

```json
{"level":"error","msg":"Failed to create destination directory"}
```

**Solutions:**

- Verify SSH user has write permissions on destination server
- Check `DEST_ROOT_DIR` path exists and is accessible
- Ensure sufficient disk space on destination

### Plex Token Invalid

```json
{"level":"error","msg":"Unauthorized: Invalid token"}
```

**Solutions:**

- Regenerate Plex token following the guide above
- Verify token has access to required libraries
- Check Plex server is accessible from container

</details>

<details>
<summary><strong>📊 Debug Mode</strong></summary>

Enable detailed logging for troubleshooting:

```yaml
environment:
  LOG_LEVEL: "DEBUG"
  DRY_RUN: "true"  # Test without making changes
```

**Debug logs include:**

- Individual file transfer progress
- SSH command execution details
- Plex API request/response details
- Metadata comparison results
- Directory creation attempts

</details>

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **Plex API**: Direct integration with Plex Media Server API
- **[logrus](https://github.com/sirupsen/logrus)**: Structured logging framework
- **Go SSH Libraries**: Secure file transfer capabilities
- **rsync**: High-performance file synchronization
- **Docker**: Containerization and deployment
