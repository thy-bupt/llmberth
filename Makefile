BINARY := llmberth
MODULE  := github.com/thy-bupt/llmberth

GO      ?= go
LINT    ?= golangci-lint

.PHONY: build test lint tidy vet golden e2e clean

build:
	$(GO) build -o $(BINARY) ./cmd/llmberth

test:
	$(GO) test ./...

golden: build
	$(GO) test -tags golden ./goldens/...

e2e: build
	./scripts/e2e.sh

lint:
	$(LINT) run

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BINARY) dist goldens/output
