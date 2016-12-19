#!/usr/bin/env bash

go run main.go instances add dev
go run main.go serve &
sleep 3
cd integration-tests/pouchdb
npm install
npm run test
testresult=$?

pidstack=$(jobs -pr)
[ -n "$pidstack" ] && kill -9 $pidstack

exit $testresult
