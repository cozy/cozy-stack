#!/bin/bash
#
# Start OnlyOffice Document Server for local development
#
# This script:
# 1. Stops and removes any existing onlyoffice-ds container
# 2. Collects all instance hostnames from cozy-stack
# 3. Starts OnlyOffice with proper host mappings so it can reach cozy-stack
#
# Usage: ./scripts/start-oo.sh
#
# See docs/office.md for more information

set -e

CONTAINER_NAME="onlyoffice-ds"
PORT="${OO_PORT:-8000}"

echo "Stopping existing OnlyOffice container..."
docker stop $CONTAINER_NAME 2>/dev/null || true
docker rm $CONTAINER_NAME 2>/dev/null || true

echo "Collecting instance hostnames..."
HOSTS=""
for domain in $(go run . instances ls 2>/dev/null | awk '{print $1}' | sed 's/:8080//'); do
  HOSTS="$HOSTS --add-host=$domain:host-gateway"
done

if [ -z "$HOSTS" ]; then
  echo "Warning: No instances found. Adding default cozy.localhost"
  HOSTS="--add-host=cozy.localhost:host-gateway"
fi

echo "Starting OnlyOffice Document Server on port $PORT..."
docker run -d --name $CONTAINER_NAME \
  -p $PORT:80 \
  -e JWT_ENABLED=false \
  -e ALLOW_PRIVATE_IP_ADDRESS=true \
  -e ALLOW_META_IP_ADDRESS=true \
  $HOSTS \
  onlyoffice/documentserver:latest

echo "Waiting for OnlyOffice to be ready..."
for i in {1..60}; do
  if curl -s http://localhost:$PORT/healthcheck 2>/dev/null | grep -q "true"; then
    echo "OnlyOffice is ready at http://localhost:$PORT"
    exit 0
  fi
  sleep 2
  echo -n "."
done

echo ""
echo "Warning: OnlyOffice did not become ready in time. Check logs with:"
echo "  docker logs $CONTAINER_NAME"
