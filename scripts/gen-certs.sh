#!/usr/bin/env bash
# Generate a self-signed certificate for local TLS testing of the gRPC server.
# The cert is valid for localhost and 127.0.0.1 for 365 days.
#
# Usage: ./scripts/gen-certs.sh [output-dir]   (default: ./certs)
set -euo pipefail

OUT_DIR="${1:-certs}"
mkdir -p "$OUT_DIR"

openssl req -x509 -newkey rsa:4096 -sha256 -nodes -days 365 \
  -keyout "$OUT_DIR/server.key" \
  -out "$OUT_DIR/server.crt" \
  -subj "/CN=gokvs" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1"

chmod 600 "$OUT_DIR/server.key"
echo "Wrote $OUT_DIR/server.crt and $OUT_DIR/server.key"
