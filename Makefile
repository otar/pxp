GO ?= go
GOFLAGS ?= -mod=readonly
VERSION ?= dev
COMMIT ?= unknown
STATICCHECK_VERSION ?= v0.7.0
GOVULNCHECK_VERSION ?= v1.6.0
ACTIONLINT_VERSION ?= v1.7.12
LDFLAGS = -s -w -X github.com/otar/pxp/internal/version.Version=$(VERSION) -X github.com/otar/pxp/internal/version.Commit=$(COMMIT)

.PHONY: all build test test-race vet fmt staticcheck actionlint vuln check clean

all: check

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/pxp ./cmd/pxp
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/pxp-convert ./cmd/pxp-convert
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/pxp-bench ./cmd/pxp-bench

test:
	$(GO) test $(GOFLAGS) ./...

test-race:
	$(GO) test $(GOFLAGS) -race ./...

vet:
	$(GO) vet $(GOFLAGS) ./...

fmt:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo 'run gofmt on the files above' >&2; exit 1; }

staticcheck:
	$(GO) run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

actionlint:
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION) -oneline .github/workflows/*.yml

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check: fmt vet staticcheck actionlint test test-race

clean:
	rm -rf bin dist coverage.out
