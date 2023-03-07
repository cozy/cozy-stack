#!/bin/bash
set -e

arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

/home/erwan/.nvm/versions/node/v16.16.0/bin/node "${arg}" | tee -a ~/.cozy/services.log
