FROM golang:1.26-alpine AS builder
ARG GOPROXY=direct
ENV GOPROXY=${GOPROXY}
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o backend-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -o publish-worker ./cmd/publish-worker

FROM golang:1.26-alpine AS dev
ARG GOPROXY=direct
ENV GOPROXY=${GOPROXY}
WORKDIR /app
# Install chromium and dependencies for headless publishing
RUN apk add --no-cache \
    git \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    tzdata

RUN go install github.com/air-verse/air@v1.65.3
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8080
CMD ["air", "--build.cmd", "go build -o /tmp/backend-api ./cmd/api", "--build.entrypoint", "/tmp/backend-api", "--build.exclude_dir", "tmp,vendor"]

FROM alpine:latest
# Install chromium and dependencies in the final image as well
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    tzdata
WORKDIR /root/
COPY --from=builder /app/backend-api .
COPY --from=builder /app/publish-worker .
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/ready >/dev/null || exit 1
CMD ["./backend-api"]
