VERSION := 0.2.0
LDFLAGS := -s -w -X main.version=$(VERSION)
BINARY  := reach

.PHONY: build test lint clean install cross

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/reach

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/

install: build
	cp bin/$(BINARY) /usr/local/bin/$(BINARY)

cross:
	GOOS=linux  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 ./cmd/reach
	GOOS=linux  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64 ./cmd/reach
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 ./cmd/reach
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 ./cmd/reach
