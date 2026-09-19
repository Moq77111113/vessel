build:
	go build -o vessel ./cmd/vessel

test:
	gofmt -l . && go vet ./... && go test ./...

.PHONY: build test
