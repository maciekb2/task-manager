# Project Environment Overview

This document explains how the **Asynchronous Task Manager** repository is structured and how the main components work together.

## Repository Structure

- **client** – Go-based gRPC client that generates example tasks and streams their status.
- **server** – gRPC gateway that accepts tasks and streams status updates.
- **ingest** – Validates tasks and performs enrichment via the enricher service.
- **enricher** – HTTP service that simulates external data enrichment.
- **scheduler** – Moves validated tasks to a priority queue (Redis sorted set).
- **worker** – Processes tasks asynchronously and emits results/status updates.
- **result-store** – Persists results in Redis and exposes a read endpoint.
- **notifier** – Updates task status in Redis and publishes status events.
- **audit** – Consumes audit events and stores them in Redis.
- **deadletter** – Captures failed tasks for later inspection.
- **pkg/flow** – Shared message definitions and queue names used across services.
- **proto** – Protocol Buffer definitions and generated Go stubs.
- **configs** – Loki, Promtail, Tempo, and OpenTelemetry Collector configs plus Grafana provisioning.
- **docker-compose.yml** – Local development stack (all services + observability).
- **k8s** – Kubernetes manifests aligned with the docker-compose stack (use kustomize).
- **docs** – Project documentation (this directory).

## Pipeline Flow

1. **client → server (gRPC)** – `SubmitTask` accepts a task and stores metadata.
2. **server → Redis (queue:ingest)** – Task payload is queued for ingestion.
3. **ingest → enricher (HTTP)** – Task is enriched and forwarded to **queue:schedule**.
4. **scheduler → Redis (priority queues)** – Task is routed to `queue:worker:*` by priority.
5. **worker → Redis (queue:results)** – Worker processes the task and emits a result.
6. **result-store → Redis (task_result:<id>)** – Result is persisted.
7. **status updates → notifier → Redis pubsub** – Status updates are published to `task_status:<id>`.
8. **server → client (gRPC stream)** – Client receives live status updates.
9. **audit/deadletter** – Background consumers store audit events and failed tasks.

Trace context is propagated via a `traceparent` field inside the task payload between async steps.

## Redis Keys and Queues

- `queue:ingest`, `queue:schedule`, `queue:results`, `queue:status`, `queue:audit`, `queue:deadletter`
- `queue:worker:high`, `queue:worker:medium`, `queue:worker:low`
- `task:<id>` (task metadata)
- `task_status:<id>` (status hash + pubsub channel)
- `task_result:<id>` (results)

## Protobuf Compilation

Protocol Buffers are defined in `proto/taskmanager.proto`. Run `make protoc` to regenerate the Go bindings if you modify the proto file.

## Running Locally

Ensure you have Docker installed. Start everything with:

```sh
docker compose up --build
```

Useful endpoints:
- Grafana: `http://localhost:3000` (admin/admin)
- Tempo API: `http://localhost:3200`
- Loki API: `http://localhost:3100`

## Kubernetes Deployment

The `k8s/` directory now mirrors the docker-compose stack (gateway, pipeline services, and observability). Apply with:

```sh
kubectl apply -k k8s
```

The manifests assume locally built images (tagged `*:local`) and use `imagePullPolicy: IfNotPresent`.
