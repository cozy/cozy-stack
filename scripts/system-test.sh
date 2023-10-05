#!/usr/bin/env bash
set -ev

cd tests/system
sudo npm install -g @bitwarden/cli@1.16.0

bundle install --jobs=3 --retry=3
# bundle exec ruby -e 'Dir.glob("tests/*.rb") { |f| load f }'
go run parallel-runner.go -fail-fast -shuffle
