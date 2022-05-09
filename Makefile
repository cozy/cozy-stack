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
	@go run . serve --mailhog --fs-url=file://localhost${PWD}/storage --konnectors-cmd ./scripts/konnector-node-run.sh
.PHONY: run

## instance: create an instance for local development
instance:
	@cozy-stack instances add cozy.localhost:8080 --passphrase cozy --apps home,store,drive,photos,settings,contacts,notes,passwords --email claude@cozy.localhost --locale fr --public-name Claude --context-name dev

## lint: enforce a consistent code style and detect code smells
lint: scripts/golangci-lint
	@scripts/golangci-lint run -E gofmt -E unconvert -E misspell -E whitespace -E exportloopref -E bidichk -D unused --max-same-issues 10 --timeout 3m0s --verbose
.PHONY: lint

scripts/golangci-lint: Makefile
	@curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b ./scripts v1.46.0

## jslint: enforce a consistent code style for Js code
jslint: scripts/node_modules
	@scripts/node_modules/.bin/eslint "assets/scripts/**" tests/integration/konnector/*.js
.PHONY: jslint

## pretty: make the assets prettier
pretty: scripts/node_modules
	@scripts/node_modules/.bin/prettier --write --no-semi --single-quote assets/*/*.js
	@scripts/node_modules/.bin/prettier --write assets/*/*.css
.PHONY: pretty

## svgo: optimize the SVG
svgo: scripts/node_modules
	@scripts/node_modules/.bin/svgo -r -f assets/icons
	@scripts/node_modules/.bin/svgo -r -f assets/images --exclude relocation-animated.svg

scripts/node_modules: Makefile scripts/package.json scripts/yarn.lock
	@cd scripts && yarn

## assets: package the assets as go code
assets: web/statik/statik.go
	@if ! [ -x "$$(command -v statik)" ]; then go install github.com/cozy/cozy-stack/pkg/statik; fi
	@scripts/build.sh assets
.PHONY: assets

## assets-fast: package the assets with the fastest level of compression
assets-fast:
	@env BROTLI_LEVEL=0 ./scripts/build.sh assets
.PHONY: assets-fast

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
	@go test -p 1 -timeout 2m -short ./...
.PHONY: unit-tests

## integration-tests: run the tests
integration-tests:
	@scripts/integration-test.sh
.PHONY: integration-tests

## clean: clean the generated files and directories
clean:
	@rm -rf bin scripts/node_modules
	@go clean
.PHONY: clean

## help: print this help message
help:
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
.PHONY: help
