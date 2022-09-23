GO := go

ifdef OS_GIT_VERSION
SOURCE_GIT_TAG := ${OS_GIT_VERSION}
endif

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps-gomod.mk \
)

GO_BUILD_PACKAGES := ./cmd/...

GO_LD_EXTRAFLAGS :=-X k8s.io/component-base/version.gitMajor="1" \
                   -X k8s.io/component-base/version.gitMinor="22" \
                   -X k8s.io/component-base/version.gitVersion="v0.22.4" \
                   -X k8s.io/component-base/version.gitCommit="$(SOURCE_GIT_COMMIT)" \
                   -X k8s.io/component-base/version.buildDate="$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')" \
                   -X k8s.io/component-base/version.gitTreeState="clean" \
                   -X k8s.io/client-go/pkg/version.gitVersion="$(SOURCE_GIT_TAG)" \
                   -X k8s.io/client-go/pkg/version.gitCommit="$(SOURCE_GIT_COMMIT)" \
                   -X k8s.io/client-go/pkg/version.buildDate="$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')" \
                   -X k8s.io/client-go/pkg/version.gitTreeState="$(SOURCE_GIT_TREE_STATE)"

GO_BUILD_FLAGS :=-tags 'json1'
GO_BUILD_BINDIR :=./bin

all: tidy test-unit build
.PHONY: all

cross-build-linux-amd64:
	@rm -rf vendor/github.com/containers/storage/drivers/register/register_btrfs.go
	@rm -rf vendor/github.com/containers/storage/drivers/btrfs/btrfs.go
	+@GOOS=linux GOARCH=amd64 $(MAKE) -tags "exclude_graphdriver_btrfs btrfs_noversion" --no-print-directory build GO_BUILD_BINDIR=$(GO_BUILD_BINDIR)/linux-amd64
.PHONY: cross-build-linux-amd64

cross-build: cross-build-linux-amd64
.PHONY: cross-build

hack-build: clean
	./hack/build.sh
.PHONY: hack-build

tidy:
	$(GO) mod tidy
	$(GO) mod verify
	$(GO) mod vendor
.PHONY: tidy

clean:
	@rm -rf ./$(GO_BUILD_BINDIR)/*
	@cd test/integration && make clean
.PHONY: clean

test-unit:
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...
.PHONY: test-unit

test-e2e: build
	./test/e2e/e2e-simple.sh ./$(GO_BUILD_BINDIR)/oc-mirror
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
	$(GO) fmt ./pkg/...
	$(GO) fmt ./cmd/...
.PHONY: format

vet: 
	$(GO) vet ./pkg/...
	$(GO) vet ./cmd/...
.PHONY: vet