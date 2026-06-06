FROM rust:1.96.0-bookworm AS builder

WORKDIR /app
COPY . .
RUN cargo build --release -p content-pipeline-service

FROM rust:1.96.0-bookworm AS dev
WORKDIR /app
RUN rustup component add rustfmt clippy
RUN cargo install cargo-watch
CMD ["cargo", "watch", "-x", "run -p content-pipeline-service"]

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && groupadd --system --gid 10001 app \
    && useradd --system --uid 10001 --gid app --home /app --shell /usr/sbin/nologin app \
    && mkdir -p /app \
    && chown app:app /app \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/content-pipeline-service /usr/local/bin/content-pipeline-service

ENV CONTENT_PIPELINE_ADDR=0.0.0.0:50051
ENV HOME=/app
EXPOSE 50051

USER app
CMD ["content-pipeline-service"]
