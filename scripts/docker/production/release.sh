#!/bin/bash
set -xe

RELEASE="$(git describe --tags)"

docker build -t "cozy/cozy-stack:${RELEASE}" -f scripts/docker/production/Dockerfile .
docker push "cozy/cozy-stack:${RELEASE}"
docker tag "cozy/cozy-stack:${RELEASE}" "cozy/cozy-stack:latest"
docker push "cozy/cozy-stack:latest"
