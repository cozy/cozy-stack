#!/usr/bin/env bash

set -o pipefail

echo "" > coverage.txt
failed=false
gosrc="$(go env GOPATH)/src"

if git grep -l \
  -e 'github.com/labstack/gommon/log' \
  -e 'github.com/dgrijalva/jwt-go' \
  -e 'github.com/cozy/echo' \
  -e 'github.com/spf13/afero' \
  -- '*.go'; then
  echo "Forbidden packages"
  exit 1
fi

for d in $(go list ./pkg/... ./web/...); do
	if test -n "$(find $gosrc/$d -maxdepth 1 -name '*_test.go' -print -quit)"; then
		go test \
			-coverprofile=profile.out \
			-covermode=count \
			-coverpkg=./pkg/...,./web/... \
			-timeout 2m \
			"$d" \
			2>&1 | grep -v 'warning: no packages being tested depend on github.com/cozy/cozy-stack'
		res=$?
		if [ $res -eq 0 ]; then
			if [ -f profile.out ]; then
				cat profile.out >> coverage.txt
				rm profile.out
			fi
		else
			failed=true
		fi
	fi
done

if [ "$failed" = true ]; then
	exit 1
else
	exit 0
fi
