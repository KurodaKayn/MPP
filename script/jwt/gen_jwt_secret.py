#!/usr/bin/env python3
import secrets

def generate_jwt_secret():
    # Generate a secure 32-byte (256-bit) secret and return it as a hex string
    secret = secrets.token_hex(32)
    print(secret)

if __name__ == "__main__":
    generate_jwt_secret()
