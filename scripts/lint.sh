#!/usr/bin/env bash
set -ev

if git grep -l \
  -e 'github.com/labstack/gommon/log' \
  -e 'github.com/dgrijalva/jwt-go' \
  -e 'github.com/labstack/echo' \
  -e 'github.com/spf13/afero' \
  -e 'github.com/cozy/statik' \
  -e 'github.com/go-redis/redis' \
  -- '*.go'; then
  echo "Forbidden packages"
  exit 1
fi

go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
golangci-lint run -E gofmt -E unconvert -E misspell

npm install eslint@5.16.0 prettier eslint-plugin-prettier eslint-config-cozy-app
./node_modules/.bin/eslint "assets/scripts/**"
