# Add build flags to GOFLAGS=<flags> here.
GO := go

.PHONY: all
all: clean tidy test build

.PHONY: build
build: clean
	$(GO) build -o bin/oc-bundle ./cmd/oc-bundle

.PHONY: test
test:
	$(GO) test -coverprofile=coverage.out -race -count=1 ./pkg/...

.PHONY: tidy
tidy:
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: clean
clean:
	@rm -rf ./bin

