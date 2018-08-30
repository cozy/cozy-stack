#!/usr/bin/env bash
set -ev

if git grep -l \
  -e 'github.com/labstack/gommon/log' \
  -e 'github.com/dgrijalva/jwt-go' \
  -e 'github.com/labstack/echo' \
  -e 'github.com/spf13/afero' \
  -- '*.go'; then
  echo "Forbidden packages"
  exit 1
fi

go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
golangci-lint run -D errcheck -E gofmt
