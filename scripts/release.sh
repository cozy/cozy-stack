#!/bin/bash
set -xe

RELEASE="$(git describe --tags)"

docker build -t "cozy/cozy-app-dev:${RELEASE}" -f scripts/Dockerfile .
docker push "cozy/cozy-app-dev:${RELEASE}"
docker tag "cozy/cozy-app-dev:${RELEASE}" "cozy/cozy-app-dev:latest"
docker push "cozy/cozy-app-dev:latest"
