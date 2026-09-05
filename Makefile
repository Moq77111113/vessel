KEY ?=
LDFLAGS = $(if $(KEY),-X github.com/Moq77111113/vessel/internal/cli.publicKey=$(shell cat $(KEY)))

# build: without KEY, the binary accepts unsigned bundles. Ship one with KEY.
build:
	go build -ldflags "$(LDFLAGS)" -o vessel ./cmd/vessel

test:
	gofmt -l . && go vet ./... && go test ./...

.PHONY: build test
