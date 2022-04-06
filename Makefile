GO := go

ifdef OS_GIT_VERSION
SOURCE_GIT_TAG := ${OS_GIT_VERSION}
endif

SOURCE_GIT_COMMIT := $(shell git rev-parse HEAD)

GO_LD_EXTRAFLAGS :=-X k8s.io/component-base/version.gitMajor="0" \
                   -X k8s.io/component-base/version.gitMinor="2" \
                   -X k8s.io/component-base/version.gitVersion="v0.2.0-alpha.1" \
                   -X k8s.io/component-base/version.gitCommit="$(SOURCE_GIT_COMMIT)" \
                   -X k8s.io/component-base/version.buildDate="$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')" \
                   -X k8s.io/component-base/version.gitTreeState="clean" \
                   -X k8s.io/client-go/pkg/version.gitVersion="$(SOURCE_GIT_TAG)" \
                   -X k8s.io/client-go/pkg/version.gitCommit="$(SOURCE_GIT_COMMIT)" \
                   -X k8s.io/client-go/pkg/version.buildDate="$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')" \
                   -X k8s.io/client-go/pkg/version.gitTreeState="$(SOURCE_GIT_TREE_STATE)"



GO_BUILD_FLAGS :=-tags 'json1 -mod=vendor'

all: clean tidy test-unit build
.PHONY: all

build: clean
	$(GO) build $(GO_BUILD_FLAGS) -ldflags="$(GO_LD_EXTRAFLAGS)" -o bin/oc-mirror ./cmd/oc-mirror
.PHONY: build

hack-build: clean
	./hack/build.sh
.PHONY: hack-build

tidy:
	$(GO) mod tidy
	$(GO) mod verify
	$(GO) mod vendor
.PHONY: vendor

clean:
	@rm -rf ./bin/*
	@cd test/integration && make clean
.PHONY: clean

test-unit:
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...
.PHONY: test-unit

test-e2e: build
	./test/e2e/e2e-simple.sh ./bin/oc-mirror
.PHONY: test-e2e

test-integration: hack-build
	@mkdir -p test/integration/output/clients
	@cp bin/oc-mirror test/integration/output/clients/
	@cd test/integration && make
.PHONY: test-integration

sanity: tidy format vet
	git diff --exit-code
.PHONY: sanity

publish-catalog:
	@cd test/operator && make
.PHONY: publish-catalog

format: 
	go fmt ./pkg/...
	go fmt ./cmd/...
.PHONY: format

vet: 
	go vet ./pkg/...
	go vet ./cmd/...
.PHONY: vet

