#!/usr/bin/env bash
set -ev

go test -timeout 2m ./...
