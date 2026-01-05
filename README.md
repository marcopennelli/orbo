# Orbo - Open Source Video Alarm System

<p align="center">
  <img src="assets/cover.png" alt="Orbo Cover" width="400">
</p>

Orbo is a modern, open-source video alarm system built with Go and OpenCV. It features real-time motion detection, AI-powered object detection (YOLO), and Telegram notifications. Designed as a cloud-native microservice, it can be deployed on Kubernetes, Docker, or Podman (including embedded Linux systems via [meta-container-deploy](https://github.com/marcopennelli/meta-container-deploy)).

## Features

- **React Web UI**: Modern dashboard with camera management, multi-camera grid layouts, and settings panel
- **Camera Support**: USB cameras (`/dev/video*`), HTTP endpoints, and RTSP streams
- **Motion Detection**: OpenCV-based with configurable sensitivity
- **AI Object Detection**: YOLO integration for persons, vehicles, and 80+ COCO classes
- **Face Recognition**: InsightFace-based face detection and identity matching
- **Telegram Integration**: Instant alerts with captured frames + bot commands for remote control
- **REST API**: Complete HTTP API for camera management and system control
- **Database Persistence**: SQLite for cameras, events, and configuration
- **Cloud Native**: Docker/Podman containers with Helm charts
- **Security Focused**: Non-root user, read-only filesystem, minimal privileges

## Quick Start

### Prerequisites

- Go 1.24.5+ and OpenCV 4.8.0+ (for backend development)
- Node.js 22+ (for frontend development)
- Docker or Podman (for containerized deployment)
- Kubernetes + Helm (for production)

### Local Development

```bash
git clone <repository-url> && cd orbo
go mod download
goa gen orbo/design

# Terminal 1: Start backend
go run ./cmd/orbo

# Terminal 2: Start frontend dev server
cd web/frontend && npm install && npm run dev
```

The frontend dev server runs on port 5173 and proxies API requests to the backend on port 8080.

### Docker

```bash
cd deploy
make docker    # Build (includes frontend)
make run       # Run with camera access
```

### Kubernetes (Minikube)

```bash
cd deploy
make minikube-build && make minikube-build-yolo && make minikube-build-recognition
make minikube-deploy          # CPU inference
make minikube-deploy-gpu      # GPU inference
```

To enable face recognition:
```bash
helm upgrade orbo deploy/helm/orbo --set recognition.enabled=true
```

## Frontend

The React-based web UI provides:

- **Camera Management**: Add, edit, delete cameras with live preview
- **Multi-Camera Grid**: Configurable layouts (1x1, 2x1, 2x2, 3x2, 3x3)
- **Motion Events**: Browse and filter detection events with thumbnails
- **Settings Panel**: Configure Telegram, YOLO, and detection settings
- **System Controls**: Start/stop detection, view system status

### Frontend Development

```bash
cd web/frontend
npm install        # Install dependencies
npm run dev        # Start dev server with hot reload
npm run build      # Production build
```

### Makefile Targets

```bash
make -C deploy frontend       # Build React frontend
make -C deploy frontend-dev   # Run dev server
make -C deploy frontend-clean # Clean build artifacts
make -C deploy full-build     # Build frontend + backend
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_ENABLED` | Enable JWT authentication | false |
| `AUTH_USERNAME` | Admin username | admin |
| `AUTH_PASSWORD` | Admin password (plaintext or bcrypt hash) | - |
| `JWT_SECRET` | Secret for signing tokens | (random) |
| `JWT_EXPIRY` | Token expiration duration | 24h |
| `TELEGRAM_ENABLED` | Enable Telegram notifications | false |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token | - |
| `TELEGRAM_CHAT_ID` | Telegram chat ID | - |
| `TELEGRAM_COOLDOWN` | Notification cooldown (seconds) | 30 |
| `YOLO_ENABLED` | Enable YOLO detection | false |
| `YOLO_ENDPOINT` | YOLO service URL | `http://yolo-service:8081` |
| `YOLO_DRAW_BOXES` | Draw bounding boxes | false |
| `YOLO_CLASSES_FILTER` | Classes to detect (e.g., "person,car") | all |
| `RECOGNITION_ENABLED` | Enable face recognition | false |
| `RECOGNITION_SERVICE_ENDPOINT` | Face recognition service URL | `http://recognition:8082` |
| `RECOGNITION_SIMILARITY_THRESHOLD` | Face match threshold (0.0-1.0) | 0.5 |
| `PRIMARY_DETECTOR` | Detection method (basic, yolo, dinov3) | basic |
| `DATABASE_PATH` | SQLite database path | `/app/frames/orbo.db` |
| `FRAME_DIR` | Frame storage directory | `/app/frames` |
| `FRONTEND_DIR` | React frontend build path | `/app/web/frontend/dist` |

### Helm Values

```bash
helm install orbo deploy/helm/orbo \
  --set notifications.telegram.enabled=true \
  --set notifications.telegram.botToken="your_token" \
  --set notifications.telegram.chatId="your_chat_id" \
  --set yolo.config.classesFilter="person"
```

| Section | Description |
|---------|-------------|
| `global` | Environment, log level |
| `orbo` | Main app: replicas, image, resources |
| `detection` | Primary detector, motion settings |
| `yolo` | YOLO config, GPU settings |
| `recognition` | Face recognition config |
| `notifications` | Telegram settings |
| `storage` | Persistence, database path |

### Telegram Setup

1. Message @BotFather on Telegram and use `/newbot`
2. Get chat ID from `https://api.telegram.org/bot<TOKEN>/getUpdates`
3. Configure via environment variables or Helm values

### Telegram Bot Commands

Once configured, control Orbo directly from Telegram:

| Command | Description |
|---------|-------------|
| `/status` | System status (cameras, detection, uptime) |
| `/cameras` | List all cameras with status |
| `/enable <name>` | Enable a camera |
| `/disable <name>` | Disable a camera |
| `/start_detection` | Start motion detection on all cameras |
| `/stop_detection` | Stop motion detection |
| `/snapshot <name>` | Capture and send a frame from a camera |
| `/events [limit]` | Show recent motion events (default: 5) |
| `/help` | Show available commands |

## API Reference

### Health
- `GET /healthz` - Liveness probe
- `GET /readyz` - Readiness probe

### Authentication
- `POST /api/v1/auth/login` - Authenticate and get JWT token
- `GET /api/v1/auth/status` - Check auth status

### Cameras
- `GET /api/v1/cameras` - List cameras
- `POST /api/v1/cameras` - Add camera
- `GET|PUT|DELETE /api/v1/cameras/{id}` - Manage camera
- `POST /api/v1/cameras/{id}/activate|deactivate` - Control camera
- `GET /api/v1/cameras/{id}/frame` - Get current frame

### Motion Events
- `GET /api/v1/motion/events` - List events
- `GET /api/v1/motion/events/{id}` - Get event details
- `GET /api/v1/motion/events/{id}/frame` - Get event frame

### Configuration
- `GET|PUT /api/v1/config/notifications` - Telegram settings
- `POST /api/v1/config/notifications/test` - Test notification
- `GET|PUT /api/v1/config/yolo` - YOLO settings
- `GET|PUT /api/v1/config/detection` - Detection settings

### System
- `GET /api/v1/system/status` - System status
- `POST /api/v1/system/detection/start|stop` - Control detection

### YOLO Service
- `POST /detect` - Detect objects (JSON response)
- `POST /detect/annotated` - Detect with annotated image
- `POST /detect/security` - Security-focused detection
- `GET /classes` - List supported classes

### Face Recognition Service
- `POST /detect` - Detect faces with age/gender estimation
- `POST /recognize` - Detect and identify known faces
- `POST /recognize/annotated` - Recognize with annotated image
- `POST /faces/register` - Register a new face identity
- `GET /faces` - List registered identities
- `DELETE /faces/{name}` - Remove a registered identity
- `GET /faces/{name}/image` - Get registered face image

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       React Frontend                            │
│            (Camera UI, Grid View, Events, Settings)             │
└───────────────────────────────┬─────────────────────────────────┘
                                │ HTTP
┌───────────────────────────────▼─────────────────────────────────┐
│                    Goa REST API Server                          │
│  ┌─────────┬──────────┬─────────┬──────────┬─────────────────┐  │
│  │ Health  │  Camera  │ Motion  │  Config  │     System      │  │
│  │ Service │  Service │ Service │  Service │     Service     │  │
│  └─────────┴────┬─────┴────┬────┴─────┬────┴────────┬────────┘  │
└─────────────────┼──────────┼──────────┼─────────────┼───────────┘
                  │          │          │             │
    ┌─────────────▼───┐  ┌───▼─────────────────┐  ┌───▼───────────┐
    │ Camera Manager  │  │  Stream Detector    │  │  Telegram Bot │
    │ (lifecycle,     │  │  (motion detection, │  │  (alerts with │
    │  frame capture) │  │   AI integration)   │  │   images)     │
    └────────┬────────┘  └──────────┬──────────┘  └───────────────┘
             │                      │
             │           ┌──────────┴─────────────────────┐
             │           │      Detection Engines         │
             │           ├─────────┬──────────┬───────────┤
             │           │  YOLO   │  Face    │  DINOv3   │
             │           │ Service │  Recog.  │  Service  │
             │           └─────────┴──────────┴───────────┘
             │
    ┌────────▼────────────────────────────────┐
    │          Camera Sources                 │
    │  ┌─────────┐  ┌────────┐  ┌──────────┐  │
    │  │   USB   │  │  HTTP  │  │   RTSP   │  │
    │  │ /dev/*  │  │  URLs  │  │  Streams │  │
    │  └─────────┘  └────────┘  └──────────┘  │
    └─────────────────────────────────────────┘
             │
    ┌────────▼────────────────────────────────┐
    │            SQLite Database              │
    │  (cameras, events, configuration)       │
    └─────────────────────────────────────────┘
```

## Project Structure

```
orbo/
├── cmd/orbo/          # Application entry point
├── design/            # Goa API design
├── gen/               # Generated Goa code
├── internal/
│   ├── camera/        # Camera management
│   ├── database/      # SQLite layer
│   ├── detection/     # YOLO/GPU clients
│   ├── motion/        # Motion detection
│   ├── telegram/      # Notifications
│   └── services/      # Service implementations
├── web/
│   └── frontend/      # React + Vite + TypeScript frontend
│       ├── src/
│       │   ├── api/       # API client
│       │   ├── components/# React components
│       │   ├── hooks/     # React Query hooks
│       │   └── types/     # TypeScript types
│       └── dist/          # Production build
├── yolo-service/         # YOLO detection service (Python)
├── recognition-service/  # Face recognition service (InsightFace)
└── deploy/
    ├── Dockerfile
    ├── Makefile
    └── helm/orbo/     # Helm charts
```

## Deployment Options

### Podman with Quadlet (systemd)

```ini
# /etc/containers/systemd/orbo.container
[Container]
Image=orbo:latest
PublishPort=8080:8080
AddDevice=/dev/video0:/dev/video0
Volume=/var/lib/orbo/frames:/app/frames:Z
Environment=TELEGRAM_ENABLED=true
Environment=TELEGRAM_BOT_TOKEN=your_token
Environment=YOLO_ENABLED=true

[Service]
Restart=always

[Install]
WantedBy=multi-user.target
```

### Yocto/Embedded Linux

Use [meta-container-deploy](https://github.com/marcopennelli/meta-container-deploy) for Yocto-based systems with automatic container management and systemd integration.

## Troubleshooting

**Camera not detected:**
```bash
ls -la /dev/video*
sudo usermod -a -G video $USER
```

**Permission denied:**
```bash
# Docker
sudo docker run --device=/dev/video0 ...
# Podman (rootless)
podman run --device /dev/video0 ...
```

## Face Recognition

Orbo includes integrated face recognition powered by [InsightFace](https://github.com/deepinsight/insightface). When enabled, the system automatically:

1. **Detects faces** when YOLO identifies a person in the frame
2. **Matches faces** against registered identities in the database
3. **Updates threat level** to "high" for unknown faces
4. **Enhances notifications** with face recognition results

### Setup

1. **Register faces** via the web UI (Settings → Face Management) or API
2. **Enable recognition** in Helm values or environment variables
3. Face recognition runs automatically during person detection

### UI Features

- **Event Cards**: Show face badges (✅ known, ⚠️ unknown)
- **Event Details**: Display identified names and unknown face count
- **Face Management**: Register, view, and delete face identities

### Recognition API

```bash
# Register a new face
curl -X POST http://orbo/recognition/faces/register \
  -F "name=John" -F "file=@photo.jpg"

# List registered faces
curl http://orbo/recognition/faces

# Recognize faces in an image
curl -X POST http://orbo/recognition/recognize \
  -F "file=@frame.jpg"
```

### Telegram Notifications

When face recognition is enabled, alerts include:
- ✅ Identified person names
- ❓ Count of unknown faces

## Roadmap

Planned features and improvements:

- [x] **Authentication** - JWT-based login with protected API endpoints
- [x] **Face Recognition** - InsightFace-based face detection and identity matching
- [ ] **Multi-user Support** - Multiple users with separate camera permissions
- [ ] **Recording & Playback** - Continuous recording with timeline navigation
- [ ] **Cloud Storage** - Optional backup to S3 for frames and recordings
- [ ] **Zones & Masks** - Define detection zones and ignore areas per camera
- [ ] **Scheduling** - Time-based detection rules (arm/disarm schedules)
- [ ] **Webhooks** - Custom HTTP callbacks on detection events
- [ ] **Audio Detection** - Sound-based alerts (glass breaking, alarms)
- [ ] **License Plate Recognition** - ALPR integration for vehicle identification

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Submit a pull request

## License

MIT License - see LICENSE file for details.

---

> Built with ❤️ in Puglia, Italy by [Marco Pennelli](https://github.com/marcopennelli)
