#!/bin/bash
set -xe

RELEASE="$(git describe --tags)"

go get -u -v ./...

docker build -t "cozy/cozy-app-dev:${RELEASE}" -f scripts/Dockerfile .
docker push "cozy/cozy-app-dev:${RELEASE}"
docker tag "cozy/cozy-app-dev:${RELEASE}" cozy/cozy-app-dev
docker push cozy/cozy-app-dev

GOOS=linux   GOARCH=amd64 ./scripts/build.sh release
GOOS=linux   GOARCH=arm   ./scripts/build.sh release
GOOS=freebsd GOARCH=amd64 ./scripts/build.sh release

rm -f "*.sha256"

sha256sum cozy-stack-*-${RELEASE} > "cozy-stack-${RELEASE}.sha256"
gpg --batch --yes --detach-sign -u 0x51F72B6A45D40BBE "cozy-stack-${RELEASE}.sha256"
