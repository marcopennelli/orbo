# Orbo - Open Source Video Alarm System

<p align="center">
  <img src="assets/cover.png" alt="Orbo Cover" width="400">
</p>

Orbo is a modern, open-source video alarm system built with Go and OpenCV, featuring real-time motion detection, USB camera support, and Telegram notifications. It's designed as a microservice following cloud-native principles and can be deployed on Kubernetes, Docker, or Podman (including embedded Linux systems built with Yocto using [meta-container-deploy](https://github.com/marcopennelli/meta-container-deploy)).

## Features

- **USB Camera Support**: Automatic detection and management of USB cameras (`/dev/video*`)
- **HTTP/RTSP Camera Support**: Network camera integration via HTTP image endpoints or RTSP streams
- **Real-time Motion Detection**: Advanced motion detection using OpenCV with configurable sensitivity
- **AI-Powered Object Detection**: YOLO integration for identifying persons, vehicles, and other objects
- **Bounding Box Visualization**: Configurable bounding boxes drawn on detection images for Telegram and API
- **Telegram Integration**: Instant notifications with captured frames sent directly to Telegram
- **REST API**: Complete HTTP REST API for camera management and system control
- **Database Persistence**: SQLite database for persisting cameras, events, and configuration across restarts
- **Health Monitoring**: Kubernetes-ready health check endpoints
- **Cloud Native**: Docker and Podman containers with Helm charts for easy deployment
- **Embedded Linux Support**: Podman deployment for Yocto-based embedded systems
- **Security Focused**: Runs as non-root user with minimal privileges

## Quick Start

### Prerequisites

- Go 1.24.5+
- OpenCV 4.8.0+
- Docker or Podman (for containerized deployment)
- Kubernetes + Helm (for production deployment)

### Development Setup

1. **Clone and initialize the project:**
   ```bash
   git clone <repository-url>
   cd orbo
   go mod download
   ```

2. **Install OpenCV** (system-dependent):
   ```bash
   # Ubuntu/Debian
   sudo apt-get update
   sudo apt-get install libopencv-dev

   # macOS
   brew install opencv

   # Arch Linux
   sudo pacman -S opencv
   ```

3. **Generate Goa code:**
   ```bash
   goa gen orbo/design
   goa example orbo/design
   ```

4. **Run locally:**
   ```bash
   go run ./cmd/orbo
   ```

### Docker Deployment

```bash
cd deploy

# Build the container
make docker

# Run with camera access (mounts /dev/video0, exposes port 8080)
make run
```

### Kubernetes Deployment (Minikube)

```bash
cd deploy

# Build and deploy with YOLO (CPU inference)
make minikube-build
make minikube-build-yolo
make minikube-deploy

# Or deploy with YOLO (GPU inference)
make minikube-build
make minikube-build-yolo
make minikube-deploy-gpu

# Check status and logs
make minikube-status
make minikube-logs
```

**Available deployment targets:**
- `minikube-deploy` / `minikube-deploy-cpu` - YOLO with CPU inference (uses `values-cpu.yaml`)
- `minikube-deploy-gpu` - YOLO with GPU inference (uses `values-gpu.yaml`)
- `minikube-deploy-test` - Test configuration (uses `values-test.yaml`)

## API Documentation

Orbo provides a comprehensive REST API for managing cameras and monitoring the system:

### Health Endpoints
- `GET /healthz` - Liveness probe
- `GET /readyz` - Readiness probe

### Camera Management
- `GET /api/v1/cameras` - List all cameras
- `POST /api/v1/cameras` - Add new camera
- `GET /api/v1/cameras/{id}` - Get camera details
- `PUT /api/v1/cameras/{id}` - Update camera configuration
- `DELETE /api/v1/cameras/{id}` - Remove camera
- `POST /api/v1/cameras/{id}/activate` - Start camera
- `POST /api/v1/cameras/{id}/deactivate` - Stop camera
- `GET /api/v1/cameras/{id}/frame` - Capture current frame (base64 JSON response)

### Motion Detection
- `GET /api/v1/motion/events` - List motion events
- `GET /api/v1/motion/events/{id}` - Get event details
- `GET /api/v1/motion/events/{id}/frame` - Get captured frame (base64 JSON response)

### Configuration
- `GET /api/v1/config/notifications` - Get notification settings
- `PUT /api/v1/config/notifications` - Update notification settings
- `POST /api/v1/config/notifications/test` - Send test notification

### System Control
- `GET /api/v1/system/status` - System status overview
- `POST /api/v1/system/detection/start` - Start motion detection
- `POST /api/v1/system/detection/stop` - Stop motion detection

### YOLO Service (when enabled)
The YOLO service runs as a separate pod and provides object detection endpoints:

- `GET /health` - Health check
- `GET /classes` - List supported object classes (80 COCO classes)
- `POST /detect` - Detect objects, returns JSON with bounding boxes
- `POST /detect/security` - Detect security-relevant objects (person, car, truck, etc.)
- `POST /detect/annotated` - Detect and return image with bounding boxes drawn
- `POST /detect/security/annotated` - Security detection with annotated image

**Annotated endpoints** support:
- `format=image` (default) - Returns raw JPEG with detection metadata in headers
- `format=base64` - Returns JSON with base64-encoded image and detection data
- `show_labels=true/false` - Toggle class labels on boxes
- `show_confidence=true/false` - Toggle confidence percentages

**Bounding box visualization:**
Uses YOLO's built-in `plot()` function for efficient annotation with automatic color coding by class.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | info |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token for notifications | - |
| `TELEGRAM_CHAT_ID` | Telegram chat ID for notifications | - |
| `DATABASE_PATH` | Path to SQLite database file | `/app/frames/orbo.db` |
| `FRAME_DIR` | Directory for storing captured frames | `/app/frames` |
| `YOLO_ENABLED` | Enable YOLO object detection | false |
| `YOLO_ENDPOINT` | YOLO service endpoint | `http://yolo-service:8081` |
| `YOLO_DRAW_BOXES` | Draw bounding boxes on detection images | false |
| `YOLO_CLASSES_FILTER` | Comma-separated list of classes to detect (e.g., "person,car") | - (all classes) |
| `PRIMARY_DETECTOR` | Primary detection method (basic, yolo, dinov3) | basic |

### Command Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--host` | Server host (localhost, 0.0.0.0) | localhost |
| `--http-port` | HTTP port | 8080 |
| `--secure` | Use HTTPS | false |
| `--debug` | Enable debug logging | false |

### Helm Values Structure

The Helm chart uses a structured values file with the following sections:

| Section | Description |
|---------|-------------|
| `global` | Environment name, log level |
| `orbo` | Main application: replicas, image, server config, resources, security context |
| `detection` | Detection settings: primary detector, fallback, motion parameters |
| `yolo` | YOLO service: enabled, config (confidence, classes filter, model), GPU settings |
| `dinov3` | DINOv3 service: enabled, config, GPU settings |
| `notifications` | Telegram: enabled, cooldown |
| `storage` | Persistence: enabled, size, paths for database and frames |
| `camera` | Device path, privileged mode |
| `service` | ClusterIP/NodePort, port |
| `ingress` | Enabled, className, hosts, TLS |
| `probes` | Liveness and readiness probe configuration |
| `autoscaling` | HPA settings |
| `serviceAccount` | Create, annotations, name |
| `secrets` | Telegram bot token and chat ID |

**Example: Filter YOLO to detect only persons (faster inference):**
```bash
helm install orbo deploy/helm/orbo \
  -f deploy/helm/orbo/values-cpu.yaml \
  --set yolo.config.classesFilter="person"
```

### Telegram Setup

1. **Create a Telegram bot:**
   - Message @BotFather on Telegram
   - Use `/newbot` command
   - Get your bot token

2. **Get your chat ID:**
   - Send a message to your bot
   - Visit `https://api.telegram.org/bot<TOKEN>/getUpdates`
   - Find your chat ID in the response

3. **Configure Orbo:**
   ```bash
   export TELEGRAM_BOT_TOKEN="your_bot_token_here"
   export TELEGRAM_CHAT_ID="your_chat_id_here"
   ./orbo
   ```

## Architecture

Orbo follows microservice architecture principles:

```
+---------------+    +---------------+    +---------------+    +---------------+
|   REST API    |    |    Motion     |    |   Telegram    |    |    YOLO       |
|    (Goa)      |    |   Detection   |    |     Bot       |    |   Service     |
|               |    |   (OpenCV)    |    |               |    |  (GPU/CPU)    |
+-------+-------+    +-------+-------+    +-------+-------+    +-------+-------+
        |                    |                    |                    |
        +--------------------+--------------------+--------------------+
                             |
                             v
                 +-----------+-----------+
                 |     Camera Manager    |
                 +-----------+-----------+
                             |
          +------------------+------------------+
          |                  |                  |
          v                  v                  v
     +----------+       +----------+       +----------+
     | Camera 1 |       | Camera 2 |       | Camera N |
     |/dev/vid0 |       | HTTP URL |       |RTSP strm |
     +----------+       +----------+       +----------+
                             |
                             v
                 +-----------+-----------+
                 |   SQLite Database     |
                 |  (cameras, events,    |
                 |   configuration)      |
                 +-----------------------+
```

### Key Components

- **Camera Manager**: Handles USB and network camera lifecycle and streaming
- **Motion Detector**: OpenCV-based motion detection with configurable sensitivity
- **YOLO Service**: GPU-accelerated object detection for identifying persons, vehicles, etc.
- **Telegram Bot**: Notification service with rate limiting and cooldown
- **REST API**: Goa-generated HTTP services with OpenAPI documentation
- **SQLite Database**: Persistent storage for cameras, events, and configuration
- **Health Checks**: Kubernetes-compatible liveness and readiness probes

## Security Considerations

### Container Security
- Runs as non-root user (UID 1000)
- Read-only root filesystem
- Minimal privileges
- No unnecessary capabilities

### API Security
- Rate limiting on endpoints
- Input validation and sanitization
- Structured error responses
- Request correlation IDs

### Camera Access
- Explicit device permissions required
- Configurable privileged mode for development
- Production deployment uses device mapping

## Development

### Project Structure
```
orbo/
├── design/                 # Goa API design
├── gen/                    # Generated Goa code
├── internal/
│   ├── camera/            # Camera management
│   ├── database/          # SQLite database layer
│   ├── detection/         # YOLO/GPU detection clients
│   ├── motion/            # Motion detection
│   ├── telegram/          # Telegram integration
│   └── services/          # Service implementations
├── cmd/orbo/              # Application entry point
├── yolo-service/          # YOLO object detection service
├── deploy/                # Deployment configurations
│   ├── Dockerfile        # Container definition
│   ├── Makefile          # Deployment automation
│   └── helm/             # Helm charts
└── docs/                  # Documentation
```

### Adding New Features

1. **Update API design** in `design/design.go`
2. **Regenerate code**: `goa gen orbo/design`
3. **Implement service** in `internal/services/`
4. **Wire up in main.go**
5. **Add tests**
6. **Update documentation**

### Testing

```bash
# Run tests
go test ./internal/...

# Run with coverage
go test -cover ./internal/...

# Integration tests (requires camera)
go test -tags integration ./...
```

## Deployment Options

### Docker Compose (Development)
```yaml
version: '3.8'
services:
  orbo:
    build:
      context: .
      dockerfile: deploy/Dockerfile
    ports:
      - "8080:8080"
    devices:
      - "/dev/video0:/dev/video0"
    volumes:
      - "./frames:/app/frames"
    environment:
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
```

### Podman Deployment

Orbo supports Podman as an alternative to Docker, making it ideal for embedded Linux systems built with Yocto.

#### Basic Podman Run
```bash
# Build the container
cd deploy
podman build -t orbo:latest -f Dockerfile ..

# Run with camera access
podman run -d \
  --name orbo \
  --device /dev/video0:/dev/video0 \
  -p 8080:8080 \
  -v ./frames:/app/frames:Z \
  -e TELEGRAM_BOT_TOKEN="your_token" \
  -e TELEGRAM_CHAT_ID="your_chat_id" \
  -e YOLO_ENABLED=true \
  -e YOLO_DRAW_BOXES=true \
  orbo:latest
```

#### Podman with Quadlet (systemd integration)

For production deployments, use Podman Quadlet for systemd integration:

```ini
# /etc/containers/systemd/orbo.container
[Unit]
Description=Orbo Video Alarm System
After=network-online.target

[Container]
Image=orbo:latest
ContainerName=orbo
PublishPort=8080:8080
AddDevice=/dev/video0:/dev/video0
Volume=/var/lib/orbo/frames:/app/frames:Z
Environment=TELEGRAM_BOT_TOKEN=your_token
Environment=TELEGRAM_CHAT_ID=your_chat_id
Environment=YOLO_ENABLED=true
Environment=YOLO_DRAW_BOXES=true

[Service]
Restart=always

[Install]
WantedBy=multi-user.target
```

#### Yocto/Embedded Linux Deployment

For deploying Orbo on Yocto-based embedded Linux systems, use [meta-container-deploy](https://github.com/marcopennelli/meta-container-deploy):

1. **Add the layer to your Yocto build:**
   ```bash
   git clone https://github.com/marcopennelli/meta-container-deploy.git
   bitbake-layers add-layer meta-container-deploy
   ```

2. **Add Orbo container to your image:**
   ```bash
   # In your local.conf or image recipe
   IMAGE_INSTALL:append = " orbo-container"
   ```

3. **Configure container deployment:**
   The meta-container-deploy layer handles:
   - Container image import during boot
   - Systemd service creation via Quadlet
   - Automatic container updates
   - Device passthrough for cameras

4. **Build and deploy:**
   ```bash
   bitbake your-image
   ```

This approach enables running Orbo on resource-constrained embedded devices with proper systemd integration and automatic startup.

### Kubernetes (Production)
```bash
# Install with custom values
helm install orbo deploy/helm/orbo \
  --set notifications.telegram.enabled=true \
  --set secrets.telegramBotToken="your_token" \
  --set secrets.telegramChatId="your_chat_id"
```

### With YOLO Object Detection
```bash
cd deploy

# Deploy with YOLO enabled (GPU mode)
make minikube-deploy-gpu

# Or directly with Helm
helm upgrade --install orbo helm/orbo \
  -f helm/orbo/values-gpu.yaml \
  --namespace orbo --create-namespace
```

## Database Persistence

Orbo uses SQLite to persist data across pod restarts. The database is stored on the same PVC as captured frames.

### What's Persisted

- **Cameras**: All camera configurations (name, device URL, resolution, FPS)
- **Motion Events**: Detection events with metadata (timestamp, confidence, bounding boxes, frame path)
- **Configuration**: Notification settings, detection parameters (excluding sensitive tokens)

### Database Schema

```sql
-- Cameras table
CREATE TABLE cameras (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    device TEXT NOT NULL,
    resolution TEXT,
    fps INTEGER DEFAULT 30,
    status TEXT DEFAULT 'inactive',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Motion events table
CREATE TABLE motion_events (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    confidence REAL,
    bounding_boxes TEXT,  -- JSON
    frame_path TEXT,
    notification_sent INTEGER DEFAULT 0,
    object_class TEXT,
    object_confidence REAL,
    threat_level TEXT,
    inference_time_ms REAL,
    detection_device TEXT
);

-- Application configuration table
CREATE TABLE app_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Database Location

By default, the database is stored at `/app/frames/orbo.db` on the same persistent volume as frame images. This can be configured via the `DATABASE_PATH` environment variable.

## Monitoring

### Metrics
- Camera status and frame rates
- Motion detection events
- Telegram notification success/failure
- API response times and error rates

### Logging
Structured logging with request correlation:
```
[orbo] 2024/03/15 10:30:15.123456 INFO[0042] request_id=abc123 msg=motion.detected camera_id=cam1 confidence=0.85
```

### Health Checks
- **Liveness**: `/healthz` - Basic service health
- **Readiness**: `/readyz` - Dependencies and camera availability

## Troubleshooting

### Common Issues

**Camera not detected:**
```bash
# Check available cameras
ls -la /dev/video*

# Test camera access (Docker)
docker run --rm -it --device=/dev/video0:/dev/video0 orbo:latest \
  sh -c "ls -la /dev/video*"

# Test camera access (Podman)
podman run --rm -it --device=/dev/video0:/dev/video0 orbo:latest \
  sh -c "ls -la /dev/video*"
```

**Permission denied:**
```bash
# Add user to video group
sudo usermod -a -G video $USER

# Or run with appropriate permissions (Docker)
sudo docker run ... orbo:latest

# Or run with appropriate permissions (Podman - rootless)
podman run --device /dev/video0 ... orbo:latest
```

**OpenCV compilation errors:**
```bash
# Install development packages
sudo apt-get install build-essential cmake pkg-config

# Check OpenCV installation
pkg-config --modversion opencv4
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Update documentation
6. Submit a pull request

## Author

**Marco Pennelli**

## License

This project is licensed under the MIT License - see the LICENSE file for details.

---

Built with love by Marco Pennelli using Go, OpenCV, and Goa Framework.
