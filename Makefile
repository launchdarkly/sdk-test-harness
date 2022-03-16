
GORELEASER_VERSION=v0.141.0
GORELEASER_CMD=curl -sL https://git.io/goreleaser | GOPATH=$(mktemp -d) VERSION=$(GORELEASER_VERSION) bash -s -- --rm-dist

GOLANGCI_LINT_VERSION=v1.44.0

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)
EXECUTABLE=./sdk-test-harness

.PHONY: build pack-files clean test lint build-release publish-release

build: $(EXECUTABLE)

$(EXECUTABLE): *.go $(shell find . -name '*.go') $(wildcard data/files/*)
	go build

pack-files:

clean:
	go clean

test:
	go test -run=not-a-real-test ./...  # just ensures that the tests compile
	go test ./...

build-release:
	$(GORELEASER_CMD) --snapshot --skip-publish --skip-validate

publish-release:
	$(GORELEASER_CMD)

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin $(GOLANGCI_LINT_VERSION)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
