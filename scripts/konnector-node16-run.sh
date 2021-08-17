#!/bin/bash
set -e

NODE_OPTS=""
arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

/usr/bin/nodejs ${NODE_OPTS} "${arg}"
