#!/usr/bin/env python3
import argparse
import base64
import os
import secrets


APP_SECRET_KINDS = ("jwt", "cookie", "collab", "ai", "pipeline", "browser")
INFRA_SECRET_KINDS = ("db", "redis", "grafana", "postgres-exporter")
SECRET_NAMES = (*APP_SECRET_KINDS, *INFRA_SECRET_KINDS, "app", "infra", "all")


def hex_secret() -> str:
    return secrets.token_hex(32)


def cookie_key() -> str:
    key = base64.b64encode(os.urandom(24)).decode("ascii").replace("+", "-").replace("/", "_")
    if len(key) != 32:
        raise RuntimeError("generated COOKIE_ENCRYPTION_KEY must be exactly 32 characters")
    return key


def generate(kind: str) -> list[tuple[str, str]]:
    if kind == "jwt":
        return [("JWT_SECRET", hex_secret())]
    if kind == "cookie":
        return [("COOKIE_ENCRYPTION_KEY", cookie_key())]
    if kind == "collab":
        return [("COLLAB_TOKEN_SECRET", hex_secret())]
    if kind == "ai":
        return [("AI_SERVICE_INTERNAL_TOKEN", hex_secret())]
    if kind == "pipeline":
        return [("CONTENT_PIPELINE_INTERNAL_TOKEN", hex_secret())]
    if kind == "browser":
        return [("BROWSER_WORKER_INTERNAL_TOKEN", hex_secret())]
    if kind == "db":
        return [("DB_PASSWORD", hex_secret())]
    if kind == "redis":
        return [("REDIS_PASSWORD", hex_secret())]
    if kind == "grafana":
        return [("GRAFANA_ADMIN_PASSWORD", hex_secret())]
    if kind == "postgres-exporter":
        return [("POSTGRES_EXPORTER_PASSWORD", hex_secret())]
    if kind == "app":
        return [item for name in APP_SECRET_KINDS for item in generate(name)]
    if kind == "infra":
        return [item for name in INFRA_SECRET_KINDS for item in generate(name)]
    if kind == "all":
        return generate("app") + generate("infra")
    raise ValueError(f"unknown secret kind: {kind}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate MPP application secrets.")
    parser.add_argument(
        "kind",
        nargs="?",
        choices=SECRET_NAMES,
        default="all",
        help="Secret to generate. Defaults to all.",
    )
    args = parser.parse_args()

    for key, value in generate(args.kind):
        print(f"{key}={value}")


if __name__ == "__main__":
    main()
