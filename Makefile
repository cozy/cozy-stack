# # Some interesting links on Makefiles:
# https://danishpraka.sh/2019/12/07/using-makefiles-for-go.html
# https://tech.davis-hansson.com/p/make/

MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
SHELL := bash

## install: compile the code and installs in binary in $GOPATH/bin
install:
	@go install
.PHONY: install

## run: start the cozy-stack for local development
run:
	@go run . serve --mailhog
.PHONY: run

## instance: create an instance for cozy.tools:8080
instance:
	@cozy-stack instances add cozy.tools:8080 --passphrase cozy --apps home,store,drive,photos,settings,contacts,notes --email claude@cozy.tools --locale fr --public-name Claude --context-name dev

## lint: enforce a consistent code style and detect code smells
lint: bin/golangci-lint
	@bin/golangci-lint run -E gofmt -E unconvert -E misspell -E whitespace -D unused --max-same-issues 10
.PHONY: lint

bin/golangci-lint: Makefile
	@curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s v1.28.1

## jslint: enforce a consistent code style for Js code
jslint: ./node_modules/.bin/eslint
	@./node_modules/.bin/eslint "assets/scripts/**" tests/integration/konnector/*.js
.PHONY: jslint

./node_modules/.bin/eslint: Makefile
	@npm install eslint@5.16.0 prettier@2.0.1 eslint-plugin-prettier@3.1.2 eslint-config-cozy-app@1.5.0

## pretty: make the assets more prettier
pretty:
	@if ! [ -x "$$(command -v prettier)" ]; then echo "Install prettier with 'npm install -g prettier'"; exit 1; fi
	@prettier --write --no-semi --single-quote assets/*/*.{css,js}
.PHONY: pretty

## assets: package the assets as go code
assets: web/statik/statik.go
	@if ! [ -x "$$(command -v statik)" ]; then go get github.com/cozy/cozy-stack/pkg/statik; fi
	@scripts/build.sh assets
.PHONY: assets

## cli: builds the CLI documentation and shell completions
cli:
	@if ! [ -x "$$(command -v cozy-stack)" ]; then make build; fi
	@scripts/build.sh assets
	@rm -rf docs/cli/*
	@cozy-stack doc markdown docs/cli
	@cozy-stack completion bash > scripts/completion/cozy-stack.bash
	@cozy-stack completion zsh > scripts/completion/cozy-stack.zsh
	@cozy-stack completion fish > scripts/completion/cozy-stack.fish
.PHONY: cli

## unit-tests: run the tests
unit-tests:
	@go test -p 1 -timeout 2m ./...
.PHONY: unit-tests

## integration-tests: run the tests
integration-tests:
	@scripts/integration-test.sh
.PHONY: integration-tests

## clean: clean the generated files and directories
clean:
	@rm -rf bin
	@go clean
.PHONY: clean

## help: print this help message
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
.PHONY: help
