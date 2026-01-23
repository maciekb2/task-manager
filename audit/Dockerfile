FROM public.ecr.aws/docker/library/golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/audit
RUN go mod tidy
RUN go mod download
RUN go build -o audit .

CMD ["./audit"]
