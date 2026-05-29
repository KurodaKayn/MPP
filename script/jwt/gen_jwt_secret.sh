#!/bin/bash

# Generate a secure 32-byte (256-bit) secret using openssl
if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
else
    # Fallback using /dev/urandom if openssl is not available
    head -c 32 /dev/urandom | xxd -p | tr -d '\n'
    echo
fi
