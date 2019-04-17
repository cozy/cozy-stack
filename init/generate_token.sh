#!/bin/bash

DEFAULT_HOST="a.cozy.tools:8080"

if [ $# -lt 1 ]; then
    echo "Usage : $0 host doctypes"
    exit 1
fi

HOST=$1

TOKEN=$(cozy-stack instances token-cli $HOST ${@:2})
echo $TOKEN
