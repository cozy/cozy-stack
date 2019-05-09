#!/usr/bin/env bash
set -ev

dir=./tests/clone/generated
mkdir -p "$dir"
go run tests/clone/generate_tests.go $(go list -f '{{.Dir}}' ./model/...) > "$dir/clone_test.go"
go test "$dir"
