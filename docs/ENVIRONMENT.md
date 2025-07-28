# Project Environment Overview

This document explains how the **Asynchronous Task Manager** repository is structured and how the main components work together.

## Repository Structure

- **client** – Go-based gRPC client that generates example tasks and streams their status.
- **server** – Go-based gRPC server that stores tasks in Redis and processes them asynchronously.
- **proto** – Protocol Buffer definitions and generated Go stubs.
- **k8s** – Kubernetes manifests for deploying the server, client, Redis and monitoring stack (Prometheus, Grafana, Loki, Tempo, etc.).
- **configs** – Placeholder configs for Loki and Promtail.
- **docs** – Project documentation (this directory).

## Core Services

### gRPC Server

`server/server.go` implements the main service. Tasks submitted by clients are stored in Redis under `task:<id>` and queued in a Redis sorted set named `tasks`. A background goroutine reads from this queue and updates task status. Clients can subscribe to status updates using a streaming gRPC endpoint.

### gRPC Client

`client/client.go` demonstrates how to connect to the server and submit random tasks. For each task it receives progress updates via the `StreamTaskStatus` RPC.

### Redis

Redis acts as the task queue and storage backend. The server connects to the host `redis-service:6379` (as defined in the Kubernetes deployment). Task status is stored under `task_status:<id>`.

## Container Images

Both server and client have a small Dockerfile based on the official Go image. The `Makefile` includes targets to build these images and push them to Docker Hub:

```sh
make build      # run protoc and build Docker images
make push       # push images to your registry
make deploy     # apply k8s manifests and restart deployments
```

`make all` runs the above steps in sequence.

## Kubernetes Deployment

The `k8s/` directory contains manifests for deploying all components. Notable files include:

- `server-deployment.yaml` – Deployment and HPA for the gRPC server.
- `client-deployment.yaml` – Example client deployment.
- `redis-deployment.yaml` – Redis instance used by the server.
- `prometheus-deployment.yaml`, `grafana-deployment.yaml`, `tempo-deployment.yaml`, `loki-deployment.yaml`, `mimir-deployment.yaml`, and `promtail-*.yaml` – optional monitoring stack used for metrics, tracing and log aggregation.

Apply these manifests with:

```sh
kubectl apply -f k8s/
```

## Protobuf Compilation

Protocol Buffers are defined in `proto/taskmanager.proto`. Run `make protoc` to regenerate the Go bindings if you modify the proto file.

## Running Locally

Ensure you have Docker and Go installed. Build and run the server and client locally using Docker:

```sh
cd server && docker build -t task-manager-server . && docker run --network host task-manager-server
cd ../client && docker build -t task-manager-client . && docker run --network host task-manager-client
```

Alternatively, use the provided Kubernetes manifests to run the stack in a cluster.

