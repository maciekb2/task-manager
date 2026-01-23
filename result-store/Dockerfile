FROM public.ecr.aws/docker/library/golang:1.23.3

WORKDIR /app
COPY . .

WORKDIR /app/result-store
RUN go mod tidy
RUN go mod download
RUN go build -o result-store .

CMD ["./result-store"]
