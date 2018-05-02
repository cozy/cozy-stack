#!/usr/bin/env bash
set -ev

cd tests/sharing
bundle install --jobs=3 --retry=3
bundle exec ruby tests/*.rb
