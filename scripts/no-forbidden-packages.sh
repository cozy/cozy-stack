#!/usr/bin/env bash

if git grep -l \
  -e 'github.com/labstack/gommon/log' \
  -e 'github.com/dgrijalva/jwt-go' \
  -e 'github.com/cozy/echo' \
  -e 'github.com/spf13/afero' \
  -- '*.go'; then
  echo "Forbidden packages"
  exit 1
fi
