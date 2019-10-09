#!/usr/bin/env bash
set -ev

cd tests/integration
sudo npm install -g @bitwarden/cli
bundle install --jobs=3 --retry=3
# bundle exec ruby -e 'Dir.glob("tests/*.rb") { |f| load f }'
go run parallel-runner.go -fail-fast -shuffle
