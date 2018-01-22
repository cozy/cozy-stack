#!/bin/bash
set -e

rundir="${1}"
runfile="${2}"

if [ -z "${runfile}" ]; then
  runfile="./index.js"
else
  runfile="./${runfile}"
fi

cd "${rundir}"
node "${runfile}"
