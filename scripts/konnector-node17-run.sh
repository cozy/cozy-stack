#!/bin/bash
set -e

NODE_BIN="/usr/bin/nodejs"
if ! [ -x "${NODE_BIN}" ]; then
  NODE_BIN="/usr/bin/node"
fi
if ! [ -x "${NODE_BIN}" ]; then
  NODE_BIN="node"
fi

arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

${NODE_BIN} "${arg}"
