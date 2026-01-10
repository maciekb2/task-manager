FROM golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/scheduler
RUN go mod tidy
RUN go mod download
RUN go build -o scheduler .

CMD ["./scheduler"]
