#!/usr/bin/env bash
set -ev

cd tests/sharing
bundle install
bundle exec ruby tests/*.rb
