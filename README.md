# Asynchronous Task Manager

## Overview
The Asynchronous Task Manager is a Dockerized application designed to manage and execute tasks asynchronously. This project aims to provide a scalable and efficient solution for handling background tasks.

**Note:** An architectural audit and improvement plan is available in [ARCHITECTURE_AUDIT.md](ARCHITECTURE_AUDIT.md).

## Features
- Asynchronous task execution
- Scalable architecture
- Dockerized for easy deployment
- Kubernetes support for orchestration
- **gRPC API** for task management and status streaming
- OpenTelemetry instrumentation with Tempo and Prometheus exporters

## Prerequisites
- Docker
- Docker Compose (for local development)
- Kubernetes (for production deployment)

## Quick Start (Docker Compose)

The easiest way to run the stack locally (Server, Client, Redis, Tempo, Prometheus) is via Docker Compose:

```sh
docker-compose up --build
```

This will start:
- **Server** on port `50051` (gRPC) and `2222` (Metrics)
- **Client** (automatically submits tasks)
- **Redis**
- **Tempo** (Tracing)
- **Prometheus** (Metrics)

## Installation (Kubernetes)
1. Clone the repository:
    ```sh
    git clone https://github.com/yourusername/asynchronous-task-manager.git
    ```
2. Navigate to the project directory:
    ```sh
    cd asynchronous-task-manager
    ```
3. Deploy the application to Kubernetes:
    ```sh
    kubectl apply -f k8s/
    ```

## Usage
See [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) for a detailed overview of how the
project is structured and how to build and deploy the services.

## API Endpoints (gRPC)
Defined in `proto/taskmanager.proto`.
- `SubmitTask`: Submit a new task.
- `StreamTaskStatus`: Stream status updates for a specific task.

## Contributing
Contributions are welcome! Please open an issue or submit a pull request.

## License
This project is licensed under the [Apache 2.0 License](LICENSE).

## Contact
For questions or support, please contact [maciekbrzezny@gmail.com](mailto:maciekbrzezny@gmail.com).
