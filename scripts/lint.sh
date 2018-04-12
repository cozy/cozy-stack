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

go get -u github.com/alecthomas/gometalinter
gometalinter --install
gometalinter --config=.golinter ./...
