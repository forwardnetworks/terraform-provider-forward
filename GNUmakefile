default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

# The prebuilt binary, so linting does not depend on this module's Go version.
GOLANGCI_LINT_VERSION := v2.14.0

lint:
	test -x bin/golangci-lint && bin/golangci-lint version | grep -q "$(GOLANGCI_LINT_VERSION:v%=%)" || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh | sh -s -- -b bin $(GOLANGCI_LINT_VERSION)
	bin/golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate
