.PHONY: all test build clean

REGISTRY_BASE ?= quay.io/luzuccar
IMAGE_NAME ?= golang-oci-mirror
IMAGE_VERSION ?= v0.0.1

all: clean test build

build: 
	mkdir -p build
	go build -o build ./... 
	

build-dev:
	mkdir -p build
	GOOS=linux go build -ldflags="-s -w" -o build -tags real./...
	chmod 755 build/microservice
	chmod 755 build/uid_entrypoint.sh

verify:
	golangci-lint run -c .golangci.yaml --deadline=30m

test:
	mkdir -p tests/results
	go test -v -short -coverprofile=tests/results/cover.out ./...

test-integration:
	mkdir -p tests/results-integration
	go test -coverprofile=tests/results-integration/cover-additional.out  -race -count=1 ./... -run TestIntegrationAdditional
	go test -coverprofile=tests/results-integration/cover-release.out  -race -count=1 ./... -run TestIntegrationRelease
	go test -coverprofile=tests/results-integration/cover-additional.out  -race -count=1 ./... -run TestIntegrationAdditionalM2M
	go test -coverprofile=tests/results-integration/cover-release.out  -race -count=1 ./... -run TestIntegrationReleaseM2M


cover:
	go tool cover -html=tests/results/cover.out -o tests/results/cover.html

clean:
	rm -rf build/*
	go clean ./...

container:
	podman build -t  ${REGISTRY_BASE}/${IMAGE_NAME}:${IMAGE_VERSION} .

push:
	podman push --authfile=${HOME}/.docker/config.json ${REGISTRY_BASE}/${IMAGE_NAME}:${IMAGE_VERSION} 

generate:
	go generate ./internal/pkg/...
