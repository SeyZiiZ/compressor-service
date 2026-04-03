.PHONY: build run test swagger docker docker-up docker-down clean deps

# Go parameters
BINARY_NAME=server
MAIN_PATH=./cmd/server

deps:
	go mod tidy

build: deps
	CGO_ENABLED=1 go build -ldflags="-s -w" -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: deps
	go run $(MAIN_PATH)

test:
	go test ./... -v -race

swagger:
	swag init -g cmd/server/main.go -o docs

docker:
	docker build -t video-compressor .

docker-up:
	docker-compose up -d --build

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f compressor

clean:
	rm -rf bin/ data/
	go clean

fmt:
	go fmt ./...
	go vet ./...
