.PHONY: build test clean run lint

BINARY=mimir
CMD=./cmd/mimir

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./... -v

clean:
	rm -f $(BINARY)
	rm -rf ~/Mimir/Investigations/test-*

run: build
	./$(BINARY)

lint:
	go vet ./...

fmt:
	go fmt ./...

all: fmt lint test build
