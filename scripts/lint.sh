#!/usr/bin/env bash
set -ev

go get -u github.com/alecthomas/gometalinter
gometalinter --install
gometalinter --config=.golinter ./...
