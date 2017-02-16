#!/usr/bin/env bash

go run main.go serve &
sleep 5
go run main.go instances add --dev --passphrase cozytest localhost:8080

curl -H 'Accept: application/json' -H 'Content-Type: application/json' http://localhost:8080/auth/register -d '{ "redirect_uris": ["http://localhost/"], "client_name": "test", "software_id": "integration-test" }'
export TEST_TOKEN=$(go run main.go instances token-oauth localhost:8080 test io.cozy.pouchtestobject)

cd integration-tests/pouchdb
npm install && npm run test
testresult=$?

pidstack=$(jobs -pr)
[ -n "$pidstack" ] && kill -9 $pidstack

exit $testresult
