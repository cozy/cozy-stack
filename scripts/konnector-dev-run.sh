#!/bin/bash
set -e
set -o pipefail

arg="${1}"

if [ ! -f "${arg}" ] && [ ! -d "${arg}" ]; then
  >&2 echo "${arg} does not exist"
  exit 1
fi

USING_NODE_LTE_16_3_0() {
  printf '%s\nv16.3.0' $(node --version) | sort -C -V
}
if USING_NODE_LTE_16_3_0; then
  dnsArg=""
else
  # If node is >v16.3.0 then request IPv4 DNS resolutions as cozy-stack does not
  # listen to IPv6 addresses yet.
  dnsArg="--dns-result-order=ipv4first"
fi

[ ! -d ~/.cozy ] && mkdir -p ~/.cozy
node $dnsArg "${arg}" 2>&1 | tee -a ~/.cozy/services.log
