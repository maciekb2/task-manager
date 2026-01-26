module github.com/maciekb2/task-manager/visualizer

go 1.23.3

toolchain go1.24.3

require (
	github.com/gorilla/websocket v1.5.1
	github.com/maciekb2/task-manager v1.0.3
	github.com/nats-io/nats.go v1.34.1
)

require (
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	go.opentelemetry.io/otel v1.33.0 // indirect
	go.opentelemetry.io/otel/trace v1.33.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace github.com/maciekb2/task-manager => ..
