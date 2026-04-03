# Video Compressor

FFmpeg-based video and image compression microservice. Exposes a REST API, processes jobs asynchronously via RabbitMQ workers, and provides real-time progress tracking through SSE (Server-Sent Events).

## Architecture

```
Client --HTTP--> [ API (Gin) ] --publish--> [ RabbitMQ ] --consume--> [ Worker Pool ]
                      |                                                     |
                      +----(SSE/status)---> [ Redis Pub/Sub ]  <--publish---+
                      |                                                     |
                      +----(persist)-----> [ PostgreSQL ] <--(update)-------+
                                                |
                                           [ Storage ]
                                        (local filesystem)
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go |
| HTTP framework | Gin |
| Video compression | FFmpeg (via `os/exec`) |
| Image compression | FFmpeg (via `os/exec`) |
| Message broker | RabbitMQ |
| Database | PostgreSQL |
| Real-time (SSE) | Redis Pub/Sub |
| API documentation | Swagger / OpenAPI 2.0 |
| Containerization | Docker + Docker Compose |

## Quick Start

### With Docker Compose (recommended)

```bash
docker-compose up --build
```

This starts all services:
- **API**: http://localhost:8080
- **Swagger UI**: http://localhost:8080/swagger/index.html
- **RabbitMQ Management**: http://localhost:15672 (guest/guest)
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379

### Local development

Prerequisites: Go 1.25+, FFmpeg, PostgreSQL, Redis, RabbitMQ installed locally.

```bash
# Install Go dependencies
make deps

# Run the server
make run

# Or build + run
make build
./bin/server
```

## API Endpoints

| Method | Route | Description |
|--------|-------|-------------|
| `POST` | `/api/v1/jobs` | Submit a compression job |
| `GET` | `/api/v1/jobs` | List jobs (pagination, status filter) |
| `GET` | `/api/v1/jobs/:id` | Get job status |
| `GET` | `/api/v1/jobs/:id/events` | **SSE** — real-time stream (progress, completed, failed) |
| `GET` | `/api/v1/jobs/:id/download` | Download compressed file |
| `DELETE` | `/api/v1/jobs/:id` | Delete a job and its files |
| `GET` | `/api/v1/health` | Health check (DB, Redis) |

### Submit a job

**File upload:**

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -F "file=@video.mp4" \
  -F "type=video" \
  -F "preset=web-optimized"
```

**From a URL:**

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -F "source_url=https://example.com/video.mp4" \
  -F "type=video" \
  -F "preset=mobile"
```

**With custom options:**

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -F "file=@photo.jpg" \
  -F "type=image" \
  -F 'options={"quality":80,"width":1920,"img_format":"webp"}'
```

### Response

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "type": "video",
  "original_filename": "video.mp4",
  "progress": 0,
  "input_size": 104857600,
  "output_size": 0,
  "compression_ratio": 0,
  "created_at": "2026-04-03T12:00:00Z"
}
```

### Real-time progress tracking (SSE)

```bash
curl -N http://localhost:8080/api/v1/jobs/550e8400/events
```

Events received:

```
event: connected
data: {"job_id":"550e8400"}

event: progress
data: {"job_id":"550e8400","progress":45,"status":"processing"}

event: completed
data: {"job_id":"550e8400","status":"completed","download_url":"/api/v1/jobs/550e8400/download","compression_ratio":0.52}
```

The stream automatically closes after a `completed` or `failed` event.

### Download result

```bash
curl -O http://localhost:8080/api/v1/jobs/550e8400/download
```

## Compression Presets

### Video

| Preset | Codec | CRF | Max width | Use case |
|--------|-------|-----|-----------|----------|
| `web-optimized` | H.264 | 23 | 1920px | Default — good size/quality tradeoff |
| `mobile` | H.264 | 28 | 1280px | Lightweight files for mobile |
| `archive` | H.264 | 18 | original | High quality archival |
| `h265-efficient` | H.265 | 28 | 1920px | Maximum compression (HEVC) |

### Image

| Preset | Format | Quality | Max width | Use case |
|--------|--------|---------|-----------|----------|
| `web-optimized` | WebP | 80 | 1920px | Default — web images |
| `thumbnail` | JPEG | 70 | 320px | Thumbnails |
| `mobile` | WebP | 75 | 1280px | Mobile images |
| `archive` | PNG | 95 | original | Maximum quality |

## Configuration

All configuration is done through environment variables (prefix `COMPRESSOR_`):

| Variable | Default | Description |
|----------|---------|-------------|
| `COMPRESSOR_SERVER_PORT` | `8080` | HTTP port |
| `COMPRESSOR_SERVER_HOST` | `0.0.0.0` | Listen address |
| `COMPRESSOR_SERVER_MAX_UPLOAD_MB` | `500` | Max upload size (MB) |
| `COMPRESSOR_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `COMPRESSOR_DATABASE_PORT` | `5432` | PostgreSQL port |
| `COMPRESSOR_DATABASE_USER` | `compressor` | DB user |
| `COMPRESSOR_DATABASE_PASSWORD` | `compressor` | DB password |
| `COMPRESSOR_DATABASE_DBNAME` | `compressor` | Database name |
| `COMPRESSOR_DATABASE_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `COMPRESSOR_REDIS_ADDR` | `localhost:6379` | Redis address |
| `COMPRESSOR_RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ URL |
| `COMPRESSOR_STORAGE_BASE_PATH` | `./data` | Storage directory |
| `COMPRESSOR_WORKER_VIDEO_WORKERS` | `2` | Number of video workers |
| `COMPRESSOR_WORKER_IMAGE_WORKERS` | `4` | Number of image workers |

A `config.yaml` file at the project root can also be used.

## Make Commands

```bash
make deps          # go mod tidy
make build         # Compile binary to bin/
make run           # Run in development mode
make test          # Run tests
make swagger       # Regenerate Swagger docs (requires swag CLI)
make docker        # Build Docker image
make docker-up     # Start docker-compose (build + up)
make docker-down   # Stop docker-compose
make docker-logs   # Tail compressor service logs
make fmt           # Format + vet
make clean         # Clean bin/ and data/
```

## Backend Integration

### Typical flow

1. Your backend calls `POST /api/v1/jobs` with the file or URL
2. Gets the `job_id` from the response
3. Opens an SSE connection on `GET /api/v1/jobs/:id/events` to track progress
4. On receiving the `completed` event, downloads the file via `GET /api/v1/jobs/:id/download`

### Via message broker (decoupled)

The service consumes RabbitMQ messages on two queues:
- `compression.jobs.video` — video jobs
- `compression.jobs.image` — image jobs

Failed jobs are routed to `compression.jobs.dlq` (dead letter queue).

### Webhook (optional)

Pass a `webhook_url` when creating a job to receive an HTTP callback when processing completes.

## Project Structure

```
cmd/server/main.go              -- Entry point
internal/
  api/                          -- HTTP layer (handlers, SSE, router, middleware)
  broker/                       -- Message broker abstraction + RabbitMQ implementation
  worker/                       -- Worker pool + job processing logic
  compress/                     -- FFmpeg wrappers (video + image) + presets
  storage/                      -- Storage abstraction + local filesystem implementation
  model/                        -- Job model
  config/                       -- Configuration (viper)
docs/                           -- Generated Swagger docs
migrations/                     -- SQL scripts
Dockerfile                      -- Multi-stage build
docker-compose.yml              -- Full stack
Makefile                        -- Development commands
```
