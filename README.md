# Event Booking Microservices

Final project for a microservice event booking system using Python, Go, Kafka, Consul, and Vault.

## Architecture

The system has one public entry point and four business services:

```text
Client -> gateway -> auth-service
                 -> events-service
                 -> booking-service -> Kafka -> notification-service
```

Full architecture notes and Mermaid diagram are in [architecture/README.md](architecture/README.md).

## Services

| Service | Stack | Port | Responsibility |
| --- | --- | ---: | --- |
| `gateway` | FastAPI | `8000` | Public API gateway and request routing |
| `auth-service` | FastAPI | `8001` | Register, login, verify tokens, read secret from Vault |
| `events-service` | FastAPI | `8002` | Event catalog and capacity metadata |
| `booking-service` | Go | `8003` | Booking workflow and Kafka publishing |
| `notification-service` | Go | `8004` | Kafka consumer for booking notifications |

## Sync communication

HTTP REST is used for operations that need an immediate response:

- client requests enter through `gateway`;
- `gateway` calls `auth-service`, `events-service`, and `booking-service`;
- `booking-service` calls `auth-service` to verify the user token;
- `booking-service` calls `events-service` to check the event before confirming a booking.

## Async communication

Kafka is used for post-booking side effects:

- `booking-service` publishes `booking.created`;
- `notification-service` consumes `booking.created`.

This keeps notification delivery independent from the booking API latency.

## Kafka

Kafka is started by Docker Compose in KRaft mode. The topic `booking.created` is auto-created in the demo environment.

## Consul

All application services register themselves in Consul and expose `/health`. Consul UI/API is available at `http://localhost:8500`.

## Vault

Vault runs in dev mode at `http://localhost:8200` with token `dev-token`. `vault-init` writes a demo `jwt_secret` to `secret/event-booking/auth`, and `auth-service` reads it on startup.

## Trade-offs

- FastAPI is a good fit for API-heavy services with strong request validation and generated OpenAPI docs.
- Go is a good fit for booking and notification services because they coordinate IO-heavy workflows and Kafka producer/consumer loops.
- Synchronous HTTP is easy to debug but creates runtime coupling between services.
- Kafka improves resilience for side effects, replay, and retries, but adds operational complexity.
- In-memory storage keeps the project focused on architecture. Production would add separate databases and an outbox pattern for reliable event publishing.
- Consul and Vault are intentionally present as infrastructure building blocks, even though the local demo keeps configuration simple through environment variables.

## How to run

```bash
docker compose up --build
```

Open:

- Gateway OpenAPI: `http://localhost:8000/docs`
- Auth OpenAPI: `http://localhost:8001/docs`
- Events OpenAPI: `http://localhost:8002/docs`
- Consul: `http://localhost:8500`
- Vault: `http://localhost:8200`

Example requests are in [docs/api-examples.md](docs/api-examples.md).
