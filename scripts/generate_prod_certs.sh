#!/bin/bash
set -e

echo "====================================="
echo " Generating DBX Production mTLS Certs"
echo "====================================="

CERT_DIR="$(dirname "$0")/../certs"
mkdir -p "$CERT_DIR"

if [ -f "$CERT_DIR/ca.key" ]; then
    echo "Certificates already exist in $CERT_DIR. Skipping generation to prevent overwriting production keys."
    exit 0
fi

# Ask for domain if interactive, else default to DBX
DOMAIN=${1:-dbx.internal}

echo "Generating Root CA..."
openssl genrsa -out "$CERT_DIR/ca.key" 4096
openssl req -x509 -new -nodes -key "$CERT_DIR/ca.key" -sha256 -days 3650 -out "$CERT_DIR/ca.crt" -subj "/CN=DBX-Root-CA"

echo "Generating Server Certificate..."
openssl genrsa -out "$CERT_DIR/server.key" 2048
openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" -subj "/CN=$DOMAIN"
openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial -out "$CERT_DIR/server.crt" -days 365 -sha256

echo "Generating Client Certificate..."
openssl genrsa -out "$CERT_DIR/client.key" 2048
openssl req -new -key "$CERT_DIR/client.key" -out "$CERT_DIR/client.csr" -subj "/CN=dbx-client"
openssl x509 -req -in "$CERT_DIR/client.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial -out "$CERT_DIR/client.crt" -days 365 -sha256

# Clean up CSRs
rm "$CERT_DIR/server.csr" "$CERT_DIR/client.csr"

echo "✅ Production certificates generated successfully in $CERT_DIR"
echo "⚠️  Keep ca.key and server.key strictly confidential!"
