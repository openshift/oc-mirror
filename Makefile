GO := go
GO_BUILD_FLAGS := -tags=json1 -mod=vendor

.PHONY: all
all: clean tidy test-unit build

.PHONY: build
build: clean
	$(GO) build $(GO_BUILD_FLAGS) -o bin/oc-mirror ./cmd/oc-mirror

.PHONY: tidy
tidy:
	$(GO) mod tidy
	$(GO) mod verify
	$(GO) mod vendor

.PHONY: clean
clean:
	@rm -rf ./bin/*

.PHONY: test-unit
test-unit: tidy
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...

.PHONY: test-e2e
test-e2e: build
	./test/e2e-simple.sh ./bin/oc-mirror

.PHONY: test-prow
test-prow: build
	./test/e2e-prow.sh

.PHONY: sanity
sanity: tidy
	git diff --exit-code