# Add build flags to GOFLAGS=<flags> here.
GO := go

.PHONY: all
all: clean tidy test-unit build

.PHONY: build
build: clean
	$(GO) build -o bin/oc-bundle ./cmd/oc-bundle

.PHONY: tidy
tidy:
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: clean
clean:
	@rm -rf ./bin

.PHONY: test-unit
test-unit:
	$(GO) test -coverprofile=coverage.out -race -count=1 ./pkg/...

.PHONY: test-e2e
test-e2e: test-e2e-operator

.PHONY: test-e2e-operator
test-e2e-operator: build
	./test/test-operator.sh ./bin/oc-bundle
