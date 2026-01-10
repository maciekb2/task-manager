FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/worker
RUN go mod tidy
RUN go mod download
RUN go build -o worker .

CMD ["./worker"]
