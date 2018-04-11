#!/bin/bash

rundir="${1}"
# rundir="/some/test/konnector/build"

mkdir "${rundir}/.rkt"
env_file="${rundir}/.rkt/env"
uuid_file="${rundir}/.rkt/uuid"

node_image="$(dirname ${0})/nodeslim.aci"

echo "COZY_URL=${COZY_URL}" > "${env_file}"
echo "COZY_FIELDS=${COZY_FIELDS}" >> "${env_file}"
echo "COZY_PARAMETERS=${COZY_PARAMETERS}" >> "${env_file}"
echo "COZY_CREDENTIALS=${COZY_CREDENTIALS}" >> "${env_file}"
echo "COZY_LOCALE=${COZY_LOCALE}" >> "${env_file}"
echo "COZY_JOB_MANUAL_EXECUTION=${COZY_JOB_MANUAL_EXECUTION}" >> "${env_file}"

rkt_name=$(echo $COZY_JOB_ID | tr A-Z a-z | sed -e 's/[^a-z0-9\-]/-/g')

trap 'sudo rkt stop --force --uuid-file="${uuid_file}" && sudo rkt rm --uuid-file="${uuid_file}"' SIGINT SIGTERM EXIT

sudo rkt run \
  --net=host \
  --set-env-file="${env_file}" \
  --uuid-file-save="${uuid_file}" \
  --volume data,kind=host,source="${rundir}" \
  --mount volume=data,target=/usr/src/app \
  --insecure-options=image "${node_image}" \
  --cpu=100m \
  --memory=128M \
  --name "${rkt_name}" \
  --exec node \
  -- /usr/src/app/index.js
