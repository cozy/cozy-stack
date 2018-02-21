#!/usr/bin/env bash

# See https://golang.org/doc/go1.10#test
go test \
	-coverprofile=tests/coverage.txt \
	-covermode=count \
	-coverpkg=./pkg/...,./web/... \
	-vet=off \
	-timeout 2m \
	./... \
