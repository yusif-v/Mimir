.PHONY: build test clean run lint fmt build-all build-darwin build-linux build-windows

BINARY=mimir
CMD=./cmd/mimir

build:
	go build -o $(BINARY) $(CMD)

build-all: build-darwin build-linux build-windows

build-darwin:
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 $(CMD)
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 $(CMD)

build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 $(CMD)
	GOOS=linux GOARCH=arm64 go build -o $(BINARY)-linux-arm64 $(CMD)

build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe $(CMD)
	GOOS=windows GOARCH=arm64 go build -o $(BINARY)-windows-arm64.exe $(CMD)

test:
	go test ./... -v

clean:
	rm -f $(BINARY) $(BINARY)-*

run: build
	./$(BINARY)

lint:
	go vet ./...

fmt:
	go fmt ./...

all: fmt lint test build

# Generate a default config file
config: build
	./$(BINARY) config

# Generate config to a custom path
config-path: build
	./$(BINARY) config $(HOME)/.mimir/config.yaml
