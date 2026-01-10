FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/notifier
RUN go mod tidy
RUN go mod download
RUN go build -o notifier .

CMD ["./notifier"]
