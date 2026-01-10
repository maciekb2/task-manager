FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/ingest
RUN go mod tidy
RUN go mod download
RUN go build -o ingest .

CMD ["./ingest"]
