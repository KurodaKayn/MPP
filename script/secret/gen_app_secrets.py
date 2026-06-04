#!/usr/bin/env python3
import argparse
import base64
import os
import secrets


SECRET_NAMES = ("jwt", "cookie", "collab", "all")


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
    if kind == "all":
        return [
            ("JWT_SECRET", hex_secret()),
            ("COOKIE_ENCRYPTION_KEY", cookie_key()),
            ("COLLAB_TOKEN_SECRET", hex_secret()),
        ]
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
