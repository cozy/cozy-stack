#!/usr/bin/env bash

set -e
echo "" > coverage.txt

for d in $(go list ./pkg/... ./web/...); do
	go test -coverprofile=profile.out -covermode=count -coverpkg=./pkg/...,./web/... $d
	if [ -f profile.out ]; then
		cat profile.out >> coverage.txt
		rm profile.out
	fi
done
