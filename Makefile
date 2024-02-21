
GORELEASER_VERSION=v1.7.0
GORELEASER_DOWNLOAD_URL=https://github.com/goreleaser/goreleaser/releases/download/v1.7.0/goreleaser_$(shell uname)_$(shell uname -m).tar.gz
GORELEASER=./bin/goreleaser/goreleaser

GOLANGCI_LINT_VERSION=v1.56.2

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)

EXECUTABLE=./sdk-test-harness

SOURCE_FILES=*.go $(shell find . -name '*.go')
TEST_DATA_FILES=$(shell find . -name '*.go') \
	$(wildcard testdata/data-files/server-side-eval/*) \
	$(wildcard testdata/data-files/client-side-eval/*)

.PHONY: build clean test lint build-release publish-release

build: $(EXECUTABLE)

$(EXECUTABLE): $(SOURCE_FILES) $(TEST_DATA_FILES)
	go build

clean:
	go clean

test:
	go test -run=not-a-real-test ./...  # just ensures that the tests compile
	go test ./...

$(GORELEASER):
	mkdir -p ./bin/goreleaser
	curl -qL $(GORELEASER_DOWNLOAD_URL) | tar xvz -C ./bin/goreleaser

build-release: $(GORELEASER)
	$(GORELEASER) --snapshot --skip-publish --skip-validate

publish-release: $(GORELEASER)
	$(GORELEASER)

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin $(GOLANGCI_LINT_VERSION)
	touch $(LINTER_VERSION_FILE)

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
