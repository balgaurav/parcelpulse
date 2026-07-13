# ParcelPulse

> Event-driven parcel visibility, designed as a small but production-minded Go microservice system.

ParcelPulse gives customer-support and operations teams a live shipment timeline, an exception queue, and delivery-notification audit trail. It is intentionally small enough to run locally and structured enough to demonstrate service boundaries, API design, observability, and frontend integration.

## What it demonstrates

- **Go microservices** — a tracking service owns shipment state and an independent notification service owns delivery alerts.
- **API gateway** — one browser-facing entry point proxies versioned API routes and reports dependency health.
- **Event-driven workflow** — status changes emit an envelope to the notification service without coupling API consumers to implementation details.
- **React dashboard** — a responsive operations view for shipment search, service health, and timeline inspection.
- **Container-first local environment** — Docker Compose starts the complete system with health checks.
- **Engineering hygiene** — unit tests, structured logs, request IDs, sample data, and a GitHub Actions verification workflow.

## Architecture

```text
React dashboard :5173
        |
        v
Gateway :8080 --------------------> Tracking service :8081
        |                                  |
        |                                  | ShipmentEvent
        |                                  v
        +--------------------------> Notification service :8082
```

The services keep state in memory to make the demo deterministic and easy to run. The ownership boundary remains explicit so a persistent store or message broker can be introduced without changing the public API.

## Quick start

```bash
docker compose up --build
# Dashboard: http://localhost:5173
# Gateway:   http://localhost:8080/api/health
```

Try a status transition:

```bash
curl -X POST http://localhost:8080/api/shipments/PP-1042/events \
  -H 'Content-Type: application/json' \
  -d '{"status":"out_for_delivery","location":"Toronto, ON","note":"Courier is on route"}'
```

## Repository layout

```text
services/
  gateway/                Browser-facing routing and aggregate health
  tracking/               Shipment timeline and status transition rules
  notifications/          Notification audit stream
web/                      React + TypeScript operations dashboard
deploy/                   Container configuration
```

## API sketch

| Route | Responsibility |
| --- | --- |
| `GET /api/health` | Gateway and dependency health |
| `GET /api/shipments` | List shipments |
| `GET /api/shipments/:id` | Shipment timeline |
| `POST /api/shipments/:id/events` | Append a validated tracking event |
| `GET /api/notifications` | Inspect generated notification audit records |

## Next production steps

Replace in-memory stores with Postgres, publish events through NATS or Kafka, add OpenTelemetry traces, and protect the dashboard with role-based authentication.
