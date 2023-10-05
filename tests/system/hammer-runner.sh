#!/usr/bin/env bash
set -e

# Usage: ./hammer-runner.sh tests/accounts_cleaning.rb
# It will launch the same system tests in a loop until it fails (up to 100
# tries). It is useful for trying to trigger an error on an system test
# that fails on some conditions that happen sometimes, but not very often.

bundle exec ruby clean.rb
for i in $(seq 0 100)
do
	echo "==== Run $i ===="
	bundle exec ruby "$@"
done
