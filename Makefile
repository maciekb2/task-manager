IMAGE_TAG ?= local

SERVER_IMAGE = taskmanager-server
CLIENT_IMAGE = taskmanager-client
INGEST_IMAGE = taskmanager-ingest
ENRICHER_IMAGE = taskmanager-enricher
SCHEDULER_IMAGE = taskmanager-scheduler
WORKER_IMAGE = taskmanager-worker
NOTIFIER_IMAGE = taskmanager-notifier
RESULT_STORE_IMAGE = taskmanager-result-store
AUDIT_IMAGE = taskmanager-audit
DEADLETTER_IMAGE = taskmanager-deadletter

.PHONY: all build build-server build-client build-ingest build-enricher build-scheduler build-worker \
	build-notifier build-result-store build-audit build-deadletter protoc k8s-apply k8s-delete

protoc:
	protoc --go_out=. \
	    --go_opt=paths=source_relative \
	    --go-grpc_out=. \
	    --go-grpc_opt=paths=source_relative \
	    proto/taskmanager.proto

build: protoc build-server build-client build-ingest build-enricher build-scheduler build-worker \
	build-notifier build-result-store build-audit build-deadletter

build-server:
	docker build -f server/Dockerfile -t $(SERVER_IMAGE):$(IMAGE_TAG) .

build-client:
	docker build -f client/Dockerfile -t $(CLIENT_IMAGE):$(IMAGE_TAG) .

build-ingest:
	docker build -f ingest/Dockerfile -t $(INGEST_IMAGE):$(IMAGE_TAG) .

build-enricher:
	docker build -f enricher/Dockerfile -t $(ENRICHER_IMAGE):$(IMAGE_TAG) .

build-scheduler:
	docker build -f scheduler/Dockerfile -t $(SCHEDULER_IMAGE):$(IMAGE_TAG) .

build-worker:
	docker build -f worker/Dockerfile -t $(WORKER_IMAGE):$(IMAGE_TAG) .

build-notifier:
	docker build -f notifier/Dockerfile -t $(NOTIFIER_IMAGE):$(IMAGE_TAG) .

build-result-store:
	docker build -f result-store/Dockerfile -t $(RESULT_STORE_IMAGE):$(IMAGE_TAG) .

build-audit:
	docker build -f audit/Dockerfile -t $(AUDIT_IMAGE):$(IMAGE_TAG) .

build-deadletter:
	docker build -f deadletter/Dockerfile -t $(DEADLETTER_IMAGE):$(IMAGE_TAG) .

k8s-apply:
	kubectl apply -k k8s

k8s-delete:
	kubectl delete -k k8s

all: build k8s-apply

MODULES = enricher client worker server result-store deadletter scheduler notifier audit ingest

unit-tests:
	@for dir in $(MODULES); do \
		echo "Testing $$dir..."; \
		(cd $$dir && go mod tidy && go test -v ./...) || exit 1; \
	done

e2e-tests:
	docker compose up --build --exit-code-from e2e-tests

test-all: unit-tests e2e-tests
