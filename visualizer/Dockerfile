FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
# If go.sum doesn't exist yet, we can skip it or create a dummy one.
# For now, let's assume we will run go mod tidy inside the container or locally.
# But to be safe in a docker build, we usually copy sources.
# Since this is a monorepo-ish structure but with separate go.mods,
# we need to be careful.

# However, the visualizer is standalone.
RUN go mod download

COPY . .
RUN go build -o visualizer main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/visualizer .
COPY --from=builder /app/static ./static

EXPOSE 8085
CMD ["./visualizer"]
