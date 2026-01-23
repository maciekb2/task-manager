FROM public.ecr.aws/docker/library/golang:1.23-alpine

WORKDIR /app

# Copy the entire repository to satisfy the 'replace' directive in go.mod
COPY . .

WORKDIR /app/tests/e2e
RUN go mod download

CMD ["go", "test", "-v", "./..."]
