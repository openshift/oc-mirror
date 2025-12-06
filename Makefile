GO := go

ifdef OS_GIT_VERSION
	SOURCE_GIT_TAG := ${OS_GIT_VERSION}
endif

GOMODCACHE=$(shell $(GO) env GOMODCACHE)

BUILD_MACHINERY_VERSION=v0.0.0-20250414185254-3ce8e800ceda
BUILD_MACHINERY_PATH=github.com/openshift/build-machinery-go

$(shell $(GO) get -tool ${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION})

# Include the library makefile
include $(addprefix ${GOMODCACHE}/${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION}/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps-gomod.mk \
)

GO_LD_EXTRAFLAGS=$(call version-ldflags,github.com/openshift/oc-mirror/v2/internal/pkg/version)

.PHONY: all test build clean

GO_MOD_FLAGS = -mod=readonly
GO_BUILD_PACKAGES := ./cmd/oc-mirror
GO_PACKAGE = github.com/openshift/oc-mirror/v2

LIBDM_BUILD_TAG = $(shell ./hack/libdm_tag.sh)
LIBSUBID_BUILD_TAG = $(shell ./hack/libsubid_tag.sh)
BTRFS_BUILD_TAG = $(shell ./hack/btrfs_tag.sh) $(shell ./hack/btrfs_installed_tag.sh)

ifeq ($(CGO_ENABLED), 0)
	override BTRFS_BUILD_TAG = exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp
endif

GO_BUILD_FLAGS = -tags "json1 $(BTRFS_BUILD_TAG) $(LIBDM_BUILD_TAG) $(LIBSUBID_BUILD_TAG)"
GO_BUILD_BINDIR := ./bin
all: clean tidy build test

verify:
	golangci-lint run -c .golangci.yaml

vet:
	$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./...
	make -C v1 vet

test-unit:
	mkdir -p tests/results
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -short -coverprofile=tests/results/cover.out -race -count=1 ./internal/pkg/...
	make -C v1 test-unit

test-integration:
	mkdir -p tests/results-integration
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-additional.out -race -count=1 ./internal/pkg/... -run 'TestIntegrationAdditional$'
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-release.out -race -count=1 ./internal/pkg/... -run 'TestIntegrationRelease$'
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-additional.out -race -count=1 ./internal/pkg/... -run TestIntegrationAdditionalM2M
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-release.out -race -count=1 ./internal/pkg/... -run TestIntegrationReleaseM2M
	make -C v1 test-integration

tidy:
	$(GO) mod tidy
	make -C v1 tidy

sanity: tidy format vet
	make -C v1 sanity
	git diff --exit-code
.PHONY: sanity

format: verify-gofmt
	make -C v1 verify-gofmt

cover:
	$(GO) tool cover -html=tests/results/cover.out -o tests/results/cover.html

clean:
	@rm -f cmd/oc-mirror/oc-mirror-v1
	@rm -rf ./$(GO_BUILD_BINDIR)/*
	make -C v1 clean
.PHONY: clean

build:
	make -C v1 build
	@cp v1/bin/oc-mirror ./cmd/oc-mirror/data/oc-mirror-v1
	mkdir -p $(GO_BUILD_BINDIR)
	go build $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) $(GO_LD_FLAGS) -o $(GO_BUILD_BINDIR) ./...
.PHONY: build

cross-build-linux-amd64:
	+@GOOS=linux GOARCH=amd64 $(MAKE) "$(GO_BUILD_FLAGS)" --no-print-directory build GO_BUILD_BINDIR=$(GO_BUILD_BINDIR)/linux-amd64
.PHONY: cross-build-linux-amd64

cross-build-linux-ppc64le:
	+@GOOS=linux GOARCH=ppc64le $(MAKE) "$(GO_BUILD_FLAGS)" --no-print-directory build GO_BUILD_BINDIR=$(GO_BUILD_BINDIR)/linux-ppc64le
.PHONY: cross-build-linux-ppc64le

cross-build-linux-s390x:
	+@GOOS=linux GOARCH=s390x $(MAKE) "$(GO_BUILD_FLAGS)" --no-print-directory build GO_BUILD_BINDIR=$(GO_BUILD_BINDIR)/linux-s390x
.PHONY: cross-build-linux-s390x

cross-build-linux-arm64:
	+@GOOS=linux GOARCH=arm64 $(MAKE) "$(GO_BUILD_FLAGS)" --no-print-directory build GO_BUILD_BINDIR=$(GO_BUILD_BINDIR)/linux-arm64
.PHONY: cross-build-linux-arm64

cross-build: cross-build-linux-amd64 cross-build-linux-ppc64le cross-build-linux-s390x cross-build-linux-arm64
.PHONY: cross-build
