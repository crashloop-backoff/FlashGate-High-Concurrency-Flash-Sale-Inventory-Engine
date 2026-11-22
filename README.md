# FlashGate

High-Concurrency Flash-Sale & Inventory Engine built with Go (`net/http` stdlib, `pgx/v5`), PostgreSQL 16, Prometheus, and Grafana.

## Prerequisites

- Docker & Docker Compose
- Go 1.23+ (for local testing & development)

## Quick Start (Local Docker Compose)

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Start the full stack (`postgres`, `api`, `prometheus`, `grafana`):
   ```bash
   make up
   ```

3. Apply database migrations:
   ```bash
   make migrate
   ```

4. Verify service health:
   ```bash
   curl http://localhost:8080/health
   ```

5. Stop the stack:
   ```bash
   make down
   ```

## Local URLs

| Service | URL | Notes |
| :--- | :--- | :--- |
| **FlashGate API (Health)** | http://localhost:8080/health | Returns PostgreSQL & Redis status |
| **FlashGate API (Metrics)** | http://localhost:8080/metrics | Prometheus exposition format |
| **Prometheus** | http://localhost:9090 | Scrapes `api:8080/metrics` every 5s |
| **Grafana** | http://localhost:3000 | Auto-provisioned Prometheus & RED dashboard (`admin` / `admin` by default in `.env.example`) |

## Development Commands

- Run unit tests with race detector:
  ```bash
  make test
  ```
- Run linter:
  ```bash
  make lint
  ```
