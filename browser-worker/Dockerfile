FROM golang:1.26-alpine AS builder
ARG GOPROXY=direct
ENV GOPROXY=${GOPROXY}
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM golang:1.26-alpine AS dev
ARG GOPROXY=direct
ENV GOPROXY=${GOPROXY}
WORKDIR /app
RUN apk add --no-cache git
RUN go install github.com/air-verse/air@v1.65.3
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8081
CMD ["air", "--build.cmd", "go build -o /tmp/browser-worker .", "--build.entrypoint", "/tmp/browser-worker", "--build.exclude_dir", "tmp,vendor"]

FROM alpine:latest
RUN addgroup -S app -g 10001 && adduser -S app -G app -u 10001 && mkdir -p /app && chown app:app /app
WORKDIR /app
ENV HOME=/home/app
COPY --from=builder --chown=app:app /app/main .
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8081/ready >/dev/null || exit 1
USER app
CMD ["./main"]
