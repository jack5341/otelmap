

<p align="center">
  <img src="assets/otelmap.png" alt="OTELMAP" width="256" />
</p>

## OTel Map Server

OpenTelemetry traces → ClickHouse storage → Service Map API. This project ingests OTLP (HTTP and gRPC) into ClickHouse via the OpenTelemetry Collector, and exposes an API to build a service map and basic SLO-style stats from collected spans. It’s designed to be simple to run with Docker and reachable on the public internet via a Cloudflare Tunnel.

### Architecture
- NGINX reverse proxy routes domains:
  - `api.otelmap.com` → Go API server (`server:8000`)
  - `otlp.otelmap.com` → OTLP HTTP `/v1/*` to collector (`otelcollector:4318`)
    - gRPC `/opentelemetry.proto` goes direct via Cloudflared (h2c) to `otelcollector:4317`
- Cloudflared tunnel publishes both domains to the internet using a token
- OpenTelemetry Collector writes to ClickHouse tables
- Go API reads from ClickHouse and returns a service-map DTO

### Prerequisites
- Docker and Docker Compose
- Cloudflare account with a Zero Trust Tunnel and token
- DNS `api.otelmap.com` and `otlp.otelmap.com` owned and managed in Cloudflare

### Environment variables (.env.production)
Create `.env.production` in the repo root with at least:

```env
# Required
CLOUDFLARE_TUNNEL_TOKEN=YOUR_TUNNEL_TOKEN

# Optional overrides
PORT=8000
SERVICE_NAME=otel-map-server
LOG_LEVEL=info
SHUTDOWN_TIMEOUT_SECONDS=10
CLICKHOUSE_DSN=clickhouse://default:default@clickhouse:9000/default?dial_timeout=5s&compress=true
```

### Domain and Tunnel
- The compose mounts `cloudflared-config.yml` which defines:
  - `api.otelmap.com` → `http://nginx:80`
  - `otlp.otelmap.com`:
    - `/opentelemetry.proto` → `h2c://otelcollector:4317` (gRPC)
    - everything else → `http://nginx:80` (OTLP HTTP under `/v1/*`)
- In Cloudflare Zero Trust → Tunnels, create a tunnel and copy its Token into `CLOUDFLARE_TUNNEL_TOKEN`.

### Run with Docker Compose

```bash
docker compose pull
docker compose up -d
```

Services:
- clickhouse:9000 (native), 8123 (HTTP)
- otelcollector:4317 (gRPC), 4318 (HTTP), 13133 (health)
- server:8000 (internal API)
- nginx:80 (internal reverse proxy)
- cloudflared (publishes the tunnel)

After startup, the public endpoints (via Cloudflare) should be:
- `https://api.otelmap.com` (Echo API)
- `https://otlp.otelmap.com/v1/traces` (OTLP HTTP ingest)
- `https://otlp.otelmap.com/opentelemetry.proto` (OTLP gRPC ingest)

### API Endpoints (internal paths)
- `GET /api/v1/healthz`
- `GET /api/v1/readyz`
- `POST /api/v1/session-token` → returns a session token and example ingest config
- `POST /api/v1/session-events` → event pooling for listening traces
- `GET /api/v1/service-map/:session-token?start=RFC3339&end=RFC3339`

### Tracing Ingest
Use the returned token to tag spans. Example (OTLP HTTP):

```bash
curl -X POST https://api.otelmap.com/api/v1/session-token
```

The response contains:
- `ingest.otlp_http_url`: e.g., `https://otlp.otelmap.com/v1/traces`
- `ingest.header_key`: `X-OTEL-SESSION`
- `ingest.header_value`: your token
- `ingest.resource_attribute`: `{ key: "otelmap.session_token", value: <token> }`

Ensure your tracer sets the resource attribute `otelmap.session_token` so spans are associated with the session.

### Context Propagation
- NGINX forwards `traceparent`, `tracestate`, and `baggage` headers
- Echo middleware extracts W3C headers into the request context
- Global propagator is set to TraceContext + Baggage
- All spans created in handlers and `MapManager` use the incoming context for proper trace continuity

### Troubleshooting
- Tunnel: verify `cloudflared` logs and that `CLOUDFLARE_TUNNEL_TOKEN` is valid
- Ingest: `curl https://otlp.otelmap.com/v1/traces -I` should return 405/404 (collector present)
- Collector health: `curl http://localhost:13133/healthz`
- ClickHouse connectivity: `docker exec -it <clickhouse> clickhouse-client --query "SELECT 1"`
- Service map empty: ensure spans include the `otelmap.session_token` resource attribute matching your session token

### Data Retention
- **6-hour retention policy**: ALL traces and session tokens are automatically deleted every 6 hours
- Cleanup runs every 6 hours via a cron job in the `cleanup` service
- Manual cleanup: `docker exec cleanup /scripts/cleanup.sh`

### Development
Standard Go module project. Key packages:
- `cmd/server`: main entrypoint
- `internal/http`: Echo routing and middleware
- `internal/handlers`: handlers for health, session token, and service map
- `pkg/map_manager`: builds the map DTO from ClickHouse rows


### Dev Container (VS Code / Cursor)
Open this repo in a dev container with ClickHouse and the OpenTelemetry Collector running alongside a Go dev environment.

Steps:

1. Install the "Dev Containers" extension.
2. Open the repo folder and choose: Reopen in Container.
3. The compose stack defined at `.devcontainer/docker-compose.devcontainer.yml` will start:
   - Go dev container (mounted workspace, port 8000 forwarded)
   - ClickHouse (9000 native, 8123 HTTP)
   - OTEL Collector (4317 gRPC, 4318 HTTP, 13133 health)
4. Inside the container terminal:

```bash
make tidy
make build
./bin/server
```

The API listens on `http://localhost:8000` on your host.
