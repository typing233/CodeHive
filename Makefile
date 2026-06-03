.PHONY: build run clean dev migrate

BINARY=codehive
VERSION?=0.1.0

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/$(BINARY) ./cmd/codehive

run: build
	./bin/$(BINARY)

dev:
	go run ./cmd/codehive

clean:
	rm -rf bin/

test:
	go test ./...

migrate:
	go run ./cmd/codehive -migrate

fmt:
	gofmt -s -w .
	goimports -w .
