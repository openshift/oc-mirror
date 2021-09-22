GO := go
GO_BUILD_FLAGS := -tags=json1

.PHONY: all
all: clean tidy test-unit build

.PHONY: build
build: clean
	$(GO) build $(GO_BUILD_FLAGS) -o bin/oc-bundle ./cmd/oc-bundle

.PHONY: tidy
tidy:
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: clean
clean:
	@rm -rf ./bin

.PHONY: test-unit
test-unit:
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...

.PHONY: test-e2e
test-e2e: build
	./test/e2e-simple.sh ./bin/oc-bundle
