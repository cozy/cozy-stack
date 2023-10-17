#!/bin/bash
set -e

NODE_BIN="$(command -v nodejs)"
if [ -z "${NODE_BIN}" ]; then
  NODE_BIN="$(command -v node)"
fi

if ! [ -x "${NODE_BIN}" ]; then
  >&2 echo "Unable to find nodejs binary at ${NODE_BIN}, exiting..."
  exit 99
fi

NODE_VERSION="$(${NODE_BIN} --version)"
if [ "${NODE_VERSION%%.*}" = "v12" ]; then
  NODE_OPTS="--max-http-header-size=16384 --tls-min-v1.0 --http-parser=legacy"
else
  NODE_OPTS=""
fi

arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

${NODE_BIN} ${NODE_OPTS} "${arg}"
