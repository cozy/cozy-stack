#!/usr/bin/env bash
set -ev

if git grep -l \
  -e 'github.com/labstack/gommon/log' \
  -e 'github.com/dgrijalva/jwt-go' \
  -e 'github.com/cozy/statik' \
  -- '*.go'; then
  echo "Forbidden packages"
  exit 1
fi

curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s v1.18.0
bin/golangci-lint run -E gofmt -E unconvert -E misspell

npm install eslint@5.16.0 prettier eslint-plugin-prettier eslint-config-cozy-app
./node_modules/.bin/eslint "assets/scripts/**"
