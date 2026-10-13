GO := go

ifdef OS_GIT_VERSION
SOURCE_GIT_TAG := ${OS_GIT_VERSION}
endif

GOMODCACHE=$(shell $(GO) env GOMODCACHE)

BUILD_MACHINERY_VERSION=v0.0.0-20240419090851-af9c868bcf52
BUILD_MACHINERY_PATH=github.com/openshift/build-machinery-go

$(shell $(GO) get ${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION})

# Include the library makefile
include $(addprefix ${GOMODCACHE}/${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION}/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps-gomod.mk \
)

GO_MOD_FLAGS = -mod=readonly
GO_BUILD_PACKAGES := ./cmd/oc-mirror
GO_PACKAGE = github.com/openshift/oc-mirror

LIBDM_BUILD_TAG = $(shell ../hack/libdm_tag.sh)
LIBSUBID_BUILD_TAG = $(shell ../hack/libsubid_tag.sh)
BTRFS_BUILD_TAG = $(shell ../hack/btrfs_tag.sh) $(shell ../hack/btrfs_installed_tag.sh)

ifeq ($(CGO_ENABLED), 0)
	override BTRFS_BUILD_TAG = exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp
endif

GO_BUILD_FLAGS = -tags "json1 $(BTRFS_BUILD_TAG) $(LIBDM_BUILD_TAG) $(LIBSUBID_BUILD_TAG)"
GO_BUILD_BINDIR :=./bin

all: tidy test-unit build
.PHONY: all

hack-build: clean
	../hack/build.sh
.PHONY: hack-build

tidy:
	$(GO) mod tidy
.PHONY: tidy

clean:
	@rm -rf ./$(GO_BUILD_BINDIR)/*
	@cd test/integration && make clean
.PHONY: clean

test-unit:
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...
.PHONY: test-unit

test-e2e: build
	./test/e2e/e2e-simple.sh ./$(GO_BUILD_BINDIR)/oc-mirror
.PHONY: test-e2e

test-integration: hack-build
	@mkdir -p test/integration/output/clients
	@cp bin/oc-mirror test/integration/output/clients/
	@cd test/integration && make
.PHONY: test-integration

sanity: tidy verify-gofmt vet
	git diff --exit-code
.PHONY: sanity

publish-catalog:
	@cd test/operator && make
.PHONY: publish-catalog

vet:
	$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./pkg/...
	$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./cmd/...
.PHONY: vet
