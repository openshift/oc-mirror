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

LIBDM_BUILD_TAG = $(shell hack/libdm_tag.sh)
LIBSUBID_BUILD_TAG = $(shell hack/libsubid_tag.sh)
BTRFS_BUILD_TAG = $(shell hack/btrfs_tag.sh) $(shell hack/btrfs_installed_tag.sh)

ifeq ($(CGO_ENABLED), 0)
	override BTRFS_BUILD_TAG = exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp
endif

GO_BUILD_FLAGS = -tags "json1 $(BTRFS_BUILD_TAG) $(LIBDM_BUILD_TAG) $(LIBSUBID_BUILD_TAG)"
GO_BUILD_BINDIR :=./bin

all: tidy test-unit build
.PHONY: all

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

hack-build: clean
	./hack/build.sh
.PHONY: hack-build

tidy:
	$(GO) mod tidy
	make -C v2 tidy
.PHONY: tidy

clean:
	@rm -f cmd/oc-mirror/oc-mirror-v2
	@rm -rf ./$(GO_BUILD_BINDIR)/*
	@cd test/integration && make clean
	make -C v2 clean
.PHONY: clean

test-unit:
	$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/... 
	make -C v2 test-unit
.PHONY: test-unit

v2cover:
	make -C v2 cover

test-e2e: build
	./test/e2e/e2e-simple.sh ./$(GO_BUILD_BINDIR)/oc-mirror
	make -C v2 test-integration
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
	$(GO) fmt $(GO_MOD_FLAGS) ./pkg/...
	$(GO) fmt $(GO_MOD_FLAGS) ./cmd/...
	make -C v2 verify-gofmt
.PHONY: format

vet:
	$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./pkg/...
	$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./cmd/...
	make -C v2 vet
.PHONY: vet

build:
	make -C v2 build
	@cp v2/build/oc-mirror ./cmd/oc-mirror/data/oc-mirror-v2
	mkdir -p $(GO_BUILD_BINDIR)
	go build $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) $(GO_LD_FLAGS) -o $(GO_BUILD_BINDIR) ./...
.PHONY: build
