# Architecture Audit & Improvement Proposals

## 1. System Overview
The Task Manager is an asynchronous task processing system built with Go, gRPC, and Redis. It consists of a Server (API + Workers) and a Client. It is designed to be deployed on Kubernetes with a full OpenTelemetry observability stack.

## 2. Current Architecture Analysis

### 2.1 Code Structure
- **Monolithic Implementation**: The entire server logic resides in a single file `server/server.go`. This includes the gRPC handler implementation, Redis storage logic, worker pool logic, and internal event propagation.
- **Client**: Similarly, `client/client.go` mixes CLI logic, connection management, and business logic.
- **Testing**: Tests exist but rely on local execution. `server_test.go` uses `miniredis`, which is a good practice for unit testing Redis interactions.

### 2.2 Concurrency & Processing
- **Worker Pool**: The server spawns a configurable number of worker goroutines (`StartWorkers`).
- **Queueing**: Tasks are queued in a Redis Sorted Set (`tasks`) based on priority.
- **State**: Task details are stored in Redis Hashes (`task:<id>`), and status in `task_status:<id>`.

### 2.3 Observability
- **OpenTelemetry**: The project uses the OTel SDK to export traces to Tempo and metrics to Prometheus.
- **Instrumentation**: gRPC interceptors are correctly used for automatic tracing. Manual metrics/spans are minimal.

## 3. Critical Findings & Risks

### 3.1 🔴 Broken Streaming in Distributed Mode (Critical)
The current implementation of `StreamTaskStatus` relies on a local in-memory map (`s.subscribers`) to notify clients of task updates.
- **The Issue**: If the system is scaled to multiple replicas (as suggested by HPA in Kubernetes manifests), a task submitted to **Replica A** might be processed by a worker on **Replica B**.
- **The Result**: **Replica B** updates the task status in Redis but only checks its *local* subscribers map. **Replica A** (where the client is connected) never receives the update signal. The client stream hangs indefinitely.
- **Fix Required**: Implementation of a distributed messaging pattern (e.g., Redis Pub/Sub) is required to broadcast updates to all API instances.

### 3.2 Coupling of API and Workers
The API server and the Worker pool run in the same process.
- **Scaling Issue**: You cannot scale the API layer (to handle more incoming connections) independently of the Worker layer (to process more tasks). If you scale the Deployment, you get more of both, which might inefficiently use resources.
- **Resilience**: A crash in the worker logic brings down the API server.

### 3.3 Hardcoded Configuration
- Service addresses (`redis-service:6379`, `tempo:4317`) are hardcoded in the Go source.
- While some env vars exist (`WORKER_COUNT`), the lack of a comprehensive configuration loader limits flexibility across environments (Local vs Dev vs Prod).

### 3.4 Documentation Inaccuracies
- `README.md` claims the project exposes a **RESTful API**. The implementation is purely **gRPC**.
- `README.md` suggests checking `docs/ENVIRONMENT.md`, which is accurate, but the primary entry point is misleading.

## 4. Architectural Proposals

### Proposal 1: Split Services (Microservices Pattern)
Refactor the application into two separate deployable units:
1.  **Task API Service**: Handles gRPC requests (`SubmitTask`, `StreamTaskStatus`), pushes to Redis, and listens for updates.
2.  **Worker Service**: Runs the worker pool, pops from Redis, processes tasks, and publishes updates.

**Benefit**: Independent scaling, better fault isolation.

### Proposal 2: Implement Redis Pub/Sub for Updates
Replace the local `subscribers` map channel logic with Redis Pub/Sub.
1.  **Worker** publishes event `task_update:<id>` (or a global channel) when status changes.
2.  **API** subscribes to this channel when a client calls `StreamTaskStatus`.
3.  This ensures that no matter which node processes the task, the node holding the client connection receives the update.

### Proposal 3: Clean Architecture Refactoring
Restructure the codebase to separate concerns:
```
/cmd
  /server      # Main entry point for API
  /worker      # Main entry point for Worker
/internal
  /api         # gRPC Handlers
  /core        # Domain models (Task, Priority)
  /storage     # Redis implementations (Repository pattern)
  /telemetry   # OTel setup
```

### Proposal 4: Docker Compose & Local Dev Experience
(Implemented in this Audit)
Add `docker-compose.yaml` to allow developers to spin up the full stack (App + Redis + Observability) with a single command, fulfilling the project's "ready for docker compose" goal.

### Proposal 5: Enhanced Observability
- Add business metrics (e.g., `tasks_processed_total`, `task_duration_seconds`).
- Ensure Context propagation works correctly between API and Worker (passing trace IDs through Redis metadata).
