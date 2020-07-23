#!/usr/bin/env bash
set -ev

cd tests/integration
sudo npm install -g @bitwarden/cli

# XXX On GitHub actions, the integration tests were failing where websockets
# were used. The Ruby library used for websockets is faye and rely on EventMachine.
# EventMachine is an event loop, and the classical way to resolve domains is
# via the glibc, but it makes a blocking syscall, and so the event loop systems
# often reimplement DNS resolution. I don't understand why but the custom resolver
# on GitHub Actions returns an IP address on which we can't connect to the
# stack (ECONNREFUSED). Maybe, it is an IPv4 vs IPv6 thing. As a work-around, we
# force the resolution via the /etc/hosts to 127.0.0.1.
#
# See https://github.com/eventmachine/eventmachine/blob/v1.2.7/lib/em/resolver.rb
echo "127.0.0.1 alice.test.cozy.tools bob.test.cozy.tools" | sudo tee -a /etc/hosts

bundle install --jobs=3 --retry=3
# bundle exec ruby -e 'Dir.glob("tests/*.rb") { |f| load f }'
go run parallel-runner.go -fail-fast -shuffle
