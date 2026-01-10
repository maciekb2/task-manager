FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/client
RUN go mod tidy
RUN go build -o client .

CMD ["./client"]
