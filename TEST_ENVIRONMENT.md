# Running in Test Environment

This environment requires using Docker images from `public.ecr.aws` to avoid Docker Hub rate limits.

The `docker-compose.yml` has been configured to use these mirrors.

## Usage

To start the application:

```bash
docker compose up --build
```

This will build the local services and pull the required infrastructure images from ECR Public.

## Images Used

The following images are pulled from ECR Public:
- `public.ecr.aws/otel/opentelemetry-collector-contrib`
- `public.ecr.aws/prometheus/prometheus`
- `public.ecr.aws/grafana/tempo`
- `public.ecr.aws/grafana/loki`
- `public.ecr.aws/grafana/promtail`
- `public.ecr.aws/grafana/grafana`
- `public.ecr.aws/docker/library/redis`
- `public.ecr.aws/docker/library/nats`
- `public.ecr.aws/bitnami/nats-exporter`
- `public.ecr.aws/envoyproxy/envoy`
