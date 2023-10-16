#!/bin/bash

set -euo pipefail

if command -v go > /dev/null; then 
  eval "$(go env)"
  export GOROOT GOPATH
else
  export GOROOT="${GOROOT:-/tmp/go}"
  export GOPATH="${GOPATH:-/tmp/goroot}"
  export PATH="${GOPATH}/bin:$GOROOT/bin:${PATH}"
fi

cd "$(dirname $0)/../.."
if [ -f debian/changelog ]; then
  echo "ERROR: debian/changelog already exists"
  exit 1
fi

echo "============="
echo "Build package"
echo "============="
DISTRO="$(lsb_release -sc)"
EPOCH=2
if [ -z "${TAG_DESC:-}" ]; then
  TAG_DESC="$(git describe --tags)"
fi
if [ -z "${VERSION:-}" ]; then
  VERSION="$(git describe --tags --abbrev=0)"
fi
if [ -z "${RELEASE:-}" ]; then
  if echo "${TAG_DESC}" | grep -q -- '-'; then
    RELEASE=$(echo "${TAG_DESC}" | cut -d- -f2)
  else
    RELEASE="1"
  fi
fi
DEBEMAIL="Cozycloud Packaging Team <debian@cozycloud.cc>" dch --create --package cozy-stack --no-auto-nmu --force-distribution -D "${DISTRO}" -v "${EPOCH}:${VERSION}-${RELEASE}~${DISTRO}" --vendor cozy "release ${TAG_DESC} for ${DISTRO}"
dpkg-buildpackage -us -uc -ui -i -I.git -b
[ ! -d packages ] && mkdir packages
mv ../cozy-stack_* packages/
rm -f debian/changelog
