FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM golang:1.26-alpine AS dev
WORKDIR /app
RUN go install github.com/air-verse/air@v1.65.3
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8081
CMD ["air", "--build.cmd", "go build -o /tmp/browser-worker .", "--build.entrypoint", "/tmp/browser-worker", "--build.exclude_dir", "tmp,vendor"]

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8081/ready >/dev/null || exit 1
CMD ["./main"]
