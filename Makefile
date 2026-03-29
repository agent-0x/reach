VERSION := 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/reach ./cmd/reach

test:
	go test ./...

clean:
	rm -rf bin/
