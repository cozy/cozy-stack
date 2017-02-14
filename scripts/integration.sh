#!/usr/bin/env bash

go run main.go serve &
sleep 5
go run main.go instances add --dev localhost:8080

export TEST_TOKEN=$(go run main.go instances token-oauth localhost:8080 test io.cozy.pouchtestobject)

cd integration-tests/pouchdb
npm install && npm run test
testresult=$?

pidstack=$(jobs -pr)
[ -n "$pidstack" ] && kill -9 $pidstack

exit $testresult
