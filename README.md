

<p align="center">
  <img src="assets/otelmap.png" alt="OTELMAP" width="256" />
</p>

## OTel Map Server

OpenTelemetry traces → ClickHouse storage → Service Map API. This project ingests OTLP (HTTP and gRPC) into ClickHouse via the OpenTelemetry Collector, and exposes an API to build a service map and basic SLO-style stats from collected spans.

<p align="center">
  <img src="assets/otelmap.gif" alt="OTELMAP" width="100%" />
</p>

### Architecture
- NGINX reverse proxy routes traffic:
  - `/api/*` → Go API server (`server:8000`)
  - `/v1/*` → OTLP HTTP to collector (`otelcollector:4318`)
  - gRPC traffic → OTLP gRPC to collector (`otelcollector:4317`)
- OpenTelemetry Collector writes to ClickHouse tables
- Go API reads from ClickHouse and returns a service-map DTO

### Prerequisites
- Docker and Docker Compose

### Environment variables (.env.production)
Create `.env.production` in the repo root with:

```env
# Required
PORT=8000
SERVICE_NAME=otel-map-server
LOG_LEVEL=info
SHUTDOWN_TIMEOUT_SECONDS=10
CLICKHOUSE_DSN=clickhouse://default:default@clickhouse:9000/default?dial_timeout=5s&compress=true
BASE_URL=localhost
```

### Run with Docker Compose

```bash
docker compose pull
docker compose up -d
```

Services:
- clickhouse:9000 (native), 8123 (HTTP)
- otelcollector:4317 (gRPC), 4318 (HTTP), 13133 (health)
- server:8000 (internal API)
- nginx:80, 443 (reverse proxy)

After startup, the endpoints will be available at:
- `http://localhost/api/v1/*` (API endpoints)
- `http://localhost/v1/traces` (OTLP HTTP ingest)
- `http://localhost:4317` (OTLP gRPC ingest - direct to collector)

### API Endpoints
- `GET /api/v1/healthz` → health check
- `GET /api/v1/readyz` → readiness check
- `POST /api/v1/session-token` → returns a session token and example ingest config
- `GET /api/v1/session-events?token=<uuid>` → SSE endpoint for listening to trace events
- `GET /api/v1/service-map/:session-token?start=RFC3339&end=RFC3339` → get service map

### Service Map Response
- Direct, Jaeger-style service dependencies are returned as deduplicated parent→child edges.
- Per-service fields:
  - `name`, `count`, `rps`, `throughput_bps`, `error_rate`.
- Per-edge fields:
  - `source` (service), `target` (service), `rps`.
- Time window:
  - Service `rps` and edge `rps` are computed over the last 60 seconds relative to the request time.

Example

```json
{
  "services": [
    {
      "name": "frontend",
      "count": 1234,
      "rps": 12.3,
      "throughput_bps": 456789.0,
      "error_rate": 0.02
    },
    {
      "name": "backend",
      "count": 980,
      "rps": 9.8,
      "throughput_bps": 321000.0,
      "error_rate": 0.01
    }
  ],
  "edges": [
    {
      "source": { "name": "frontend", "count": 1234, "rps": 12.3, "throughput_bps": 456789.0, "error_rate": 0.02 },
      "rps": 8.5,
      "target": { "name": "backend", "count": 980, "rps": 9.8, "throughput_bps": 321000.0, "error_rate": 0.01 }
    }
  ]
}
```

Conceptually aligned with Jaeger’s service dependency graph. See: [Jaeger repository](https://github.com/jaegertracing/jaeger).

### Tracing Ingest
First, create a session token:

```bash
curl -X POST http://localhost/api/v1/session-token
```

The response contains:
- `token`: your unique session token (UUID)
- `ingest.otlp_http_url`: e.g., `http://localhost/v1/traces`
- `ingest.otlp_grpc_url`: e.g., `http://localhost:4317`
- `ingest.resource_attribute`: `{ key: "otelmap.session_token", value: <token> }`

**Important**: Ensure your tracer sets the resource attribute `otelmap.session_token` with your session token value so spans are associated with your session.

### Context Propagation
- NGINX forwards `traceparent`, `tracestate`, and `baggage` headers
- Echo middleware extracts W3C headers into the request context
- Global propagator is set to TraceContext + Baggage
- All spans created in handlers and `MapManager` use the incoming context for proper trace continuity

### Troubleshooting
- Ingest: `curl http://localhost/v1/traces -I` should return 405/404 (collector present)
- Collector health: `curl http://localhost:13133/healthz`
- ClickHouse connectivity: `docker exec -it <clickhouse-container> clickhouse-client --query "SELECT 1"`
- Service map empty: ensure spans include the `otelmap.session_token` resource attribute matching your session token
- Check collector logs: `docker logs otel`
- Check server logs: `docker logs <server-container-name>`

### Development
Standard Go module project. Key packages:
- `cmd/server`: main entrypoint
- `internal/http`: Echo routing and middleware
- `internal/handlers`: handlers for health, session token, and service map
- `pkg/map_manager`: builds the map DTO from ClickHouse rows