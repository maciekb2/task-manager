FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/server
RUN go mod tidy
RUN go mod download
RUN go build -o server .

EXPOSE 50051
CMD ["./server"]
