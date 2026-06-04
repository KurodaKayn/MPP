FROM rust:1.89-bookworm AS builder

WORKDIR /app
COPY . .
RUN cargo build --release -p content-pipeline-service

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/content-pipeline-service /usr/local/bin/content-pipeline-service

ENV CONTENT_PIPELINE_ADDR=0.0.0.0:50051
EXPOSE 50051

CMD ["content-pipeline-service"]

