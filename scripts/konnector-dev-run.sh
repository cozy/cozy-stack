#!/bin/bash
set -e
set -o pipefail

arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

[ ! -d ~/.cozy ] && mkdir -p ~/.cozy
node "${arg}" 2>&1 | tee -a ~/.cozy/services.log
