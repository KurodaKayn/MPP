FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/api/main.go

FROM golang:1.26-alpine AS dev
WORKDIR /app
# Install chromium and dependencies for headless publishing
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont

RUN go install github.com/air-verse/air@v1.65.3
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8080
CMD ["air", "--build.cmd", "go build -o /tmp/backend-api ./cmd/api/main.go", "--build.entrypoint", "/tmp/backend-api", "--build.exclude_dir", "tmp,vendor"]

FROM alpine:latest
# Install chromium and dependencies in the final image as well
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
