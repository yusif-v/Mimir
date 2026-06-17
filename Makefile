.PHONY: build test clean run lint fmt

BINARY=mimir
CMD=./cmd/mimir

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./... -v

clean:
	rm -f $(BINARY)

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
