default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --max-issues-per-linter=0 --max-same-issues=0 ./...

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate
