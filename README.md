# Asynchronous Task Manager

## Overview
The Asynchronous Task Manager is a Dockerized application designed to manage and execute tasks asynchronously. This project aims to provide a scalable and efficient solution for handling background tasks.

## Features
- Asynchronous task execution
- Scalable architecture
- Dockerized for easy deployment
- Kubernetes support for orchestration
- gRPC API gateway for task management
- OpenTelemetry instrumentation with Tempo and Prometheus exporters

## Prerequisites
- Docker
- Kubernetes (optional)

## Installation
1. Clone the repository:
    ```sh
    git clone https://github.com/yourusername/asynchronous-task-manager.git
    ```
2. Navigate to the project directory:
    ```sh
    cd asynchronous-task-manager
    ```
3. Deploy the application to Kubernetes (optional):
    ```sh
    make build
    kubectl apply -k k8s
    ```

## Usage
See [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) for a detailed overview of how the
project is structured and how to build and deploy the services.

## Local quick start
```sh
docker compose up --build
```

## API
The public API is gRPC (`proto/TaskManager`) with:
- `SubmitTask` to enqueue a task
- `StreamTaskStatus` to stream task status updates

## Contributing
Contributions are welcome! Please open an issue or submit a pull request.

## License
This project is licensed under the [Apache 2.0 License](LICENSE).

## Contact
For questions or support, please contact [maciekbrzezny@gmail.com](mailto:maciekbrzezny@gmail.com).
