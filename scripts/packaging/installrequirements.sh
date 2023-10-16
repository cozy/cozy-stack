#!/bin/bash

set -euo pipefail

GOVERSION="${GOVERSION:-1.21.3}"

cd "$(dirname $0)/../.."
if [ -f debian/changelog ]; then
  echo "ERROR: debian/changelog already exists"
  exit 1
fi

echo "===================="
echo "Install Requirements"
echo "===================="
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install --yes devscripts lsb-release binutils fakeroot quilt devscripts dpkg-dev libdistro-info-perl equivs aptitude wget build-essential git --no-install-recommends --no-install-suggests
dpkg-checkbuilddeps || (yes y | mk-build-deps debian/control -ir) || true

if [ -z "${SKIP_GO:-}" ]; then
  echo "=========="
  echo "Install GO"
  echo "=========="
  export GOROOT="${GOROOT:-/tmp/goroot}"
  [ ! -d "${GOROOT}" ] && mkdir -p "${GOROOT}"
  export GOPATH="${GOPATH:-/tmp/go}"
  [ ! -d "${GOPATH}" ] && mkdir -p "${GOPATH}"
  if [ ! -x "${GOROOT}/bin/go" ]; then
    echo ". download go archive"
    [ -f "go.tar.gz" ] || wget --quiet https://dl.google.com/go/go${GOVERSION}.linux-amd64.tar.gz -O /tmp/go.tar.gz
    echo ". extract go archive"
    [ ! -d "${GOROOT}" ] && mkdir -p "${GOROOT}"
    tar xf /tmp/go.tar.gz -C "${GOROOT}" --strip-components=1
    rm -rf /tmp/go.tar.gz
  fi
  export PATH="${GOPATH}/bin:${GOROOT}/bin:${PATH}"
fi
