#!/bin/bash

mkdir "${1}/.rkt"
env_file="${1}/.rkt/env"
uuid_file="${1}/.rkt/uuid"

node_image="$(dirname ${0})/nodeslim.aci"

echo "COZY_DOMAIN=${COZY_DOMAIN}" > "${env_file}"
echo "COZY_FIELDS=${COZY_FIELDS}" >> "${env_file}"
echo "COZY_CREDENTIALS=${COZY_CREDENTIALS}" >> "${env_file}"

trap 'sudo rkt stop --force --uuid-file="${uuid_file}" && sudo rkt rm --uuid-file="${uuid_file}"' SIGINT SIGTERM EXIT

sudo rkt run \
  --net=host \
  --set-env-file="${env_file}" \
  --uuid-file-save="${uuid_file}" \
  --volume data,kind=host,source="${1}" \
  --mount volume=data,target=/usr/src/app \
  --insecure-options=image "${node_image}" \
  --cpu=100m \
  --memory=128M \
  --name "${COZY_JOB_ID}" \
  --exec node \
  -- /usr/src/app/index.js
