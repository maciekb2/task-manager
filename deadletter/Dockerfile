FROM public.ecr.aws/docker/library/golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/deadletter
RUN go mod tidy
RUN go mod download
RUN go build -o deadletter .

CMD ["./deadletter"]
