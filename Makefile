
GOLANGCI_LINT_VERSION=v1.43.0

LINTER=./bin/golangci-lint
LINTER_VERSION_FILE=./bin/.golangci-lint-version-$(GOLANGCI_LINT_VERSION)
EXECUTABLE=./sdk-test-harness

.PHONY: build clean test lint

build: $(EXECUTABLE)

$(EXECUTABLE): *.go $(wildcard */*.go)
	go build

clean:
	go clean

test:
	go test -run=not-a-real-test ./...  # just ensures that the tests compile
	go test ./...

$(LINTER_VERSION_FILE):
	rm -f $(LINTER)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.43.0

lint: $(LINTER_VERSION_FILE)
	$(LINTER) run ./...
