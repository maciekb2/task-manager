# Zmienna z nazwą użytkownika Docker Hub
DOCKER_USER = maciekb2

# Nazwy obrazów
SERVER_IMAGE = $(DOCKER_USER)/task-manager-server
CLIENT_IMAGE = $(DOCKER_USER)/task-manager-client

# Tagowanie wersji obrazów
TAG = latest

.PHONY: all build push deploy protoc

# Generowanie plików Protobuf
protoc:
	protoc --go_out=. \
	    --go_opt=paths=source_relative \
	    --go-grpc_out=. \
	    --go-grpc_opt=paths=source_relative \
	    proto/taskmanager.proto

# Komenda do zbudowania wszystkich obrazów
build: protoc build-server build-client

build-server:
	@echo "Budowanie obrazu serwera..."
	docker build -f server/Dockerfile -t $(SERVER_IMAGE):$(TAG) .

build-client:
	@echo "Budowanie obrazu klienta..."
	docker build -f client/Dockerfile -t $(CLIENT_IMAGE):$(TAG) .

# Komenda do pushowania obrazów na Docker Hub
push: push-server push-client

push-server:
	@echo "Przesyłanie obrazu serwera na Docker Hub..."
	docker push $(SERVER_IMAGE):$(TAG)

push-client:
	@echo "Przesyłanie obrazu klienta na Docker Hub..."
	docker push $(CLIENT_IMAGE):$(TAG)

# Komenda do wdrożenia aplikacji na Kubernetes
deploy: deploy-server deploy-client

deploy-server:
	@echo "Tworzenie deploymentu serwera..."
	kubectl apply -f k8s/server-deployment.yaml
	kubectl rollout restart deployment/taskmanager-server

deploy-client:
	@echo "Tworzenie deploymentu klienta..."
	kubectl apply -f k8s/client-deployment.yaml
	kubectl rollout restart deployment/taskmanager-client

# Komenda do zbudowania, przesłania obrazów i wdrożenia aplikacji
all: build push deploy
	@echo "Wszystkie kroki zakończone sukcesem!"
