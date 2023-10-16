#!/bin/bash

TARGETS="debian:10 debian:11 debian:12 ubuntu:20.04 ubuntu:22.04"

if [ $# -ne 0 ]; then
  TARGETS="$@"
fi

SCRIPT_DIR=$(dirname $0)
STACK_DIR=$(readlink -f ${SCRIPT_DIR}/../..)

for i in ${TARGETS}; do
  echo "*** building for $i"
  [ -f "${STACK_DIR}/debian/changelog" ] && rm -f "${STACK_DIR}/debian/changelog"
  docker run --rm -v ${STACK_DIR}:/build $i /bin/bash -c 'echo "[safe]" > /root/.gitconfig && echo "        directory = /build" >> /root/.gitconfig && cd /build && scripts/packaging/installrequirements.sh && scripts/packaging/buildpackage.sh'
done

