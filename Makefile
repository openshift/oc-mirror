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

GO_LD_EXTRAFLAGS=$(call version-ldflags,github.com/openshift/oc-mirror/v2/pkg/version)

#v2 workspace required change
#GO_MOD_FLAGS = -mod=readonly
GO_BUILD_PACKAGES := ./cmd/...
#v2 workspace required change
#GO_PACKAGE = github.com/openshift/oc-mirror

LIBDM_BUILD_TAG = $(shell hack/libdm_tag.sh)
LIBSUBID_BUILD_TAG = $(shell hack/libsubid_tag.sh)
BTRFS_BUILD_TAG = $(shell hack/btrfs_tag.sh) $(shell hack/btrfs_installed_tag.sh)

ifeq ($(DISABLE_CGO), 1)
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
	$(GO) mod verify
	$(GO) mod vendor
.PHONY: tidy

clean:
	@rm -rf ./$(GO_BUILD_BINDIR)/*
	@cd test/integration && make clean
.PHONY: clean

test-unit:
#v2 workspace required change
#$(GO) test $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/... 
	$(GO) test $(GO_BUILD_FLAGS) -coverprofile=coverage.out -race -count=1 ./pkg/...
	mkdir -p v2/tests/results
	@cd v2 && $(GO) test $(GO_BUILD_FLAGS) -short -coverprofile=tests/results/cover.out -race -count=1 ./pkg/...
.PHONY: test-unit

v2cover:
	go tool cover -html=v2/tests/results/cover.out -o v2/tests/results/cover.html

test-e2e: build
	./test/e2e/e2e-simple.sh ./$(GO_BUILD_BINDIR)/oc-mirror
	mkdir -p v2/tests/results-integration
	@cd v2 && $(GO) test $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-additional.out -race -count=1 ./pkg/... -run TestIntegrationAdditional
	@cd v2 && $(GO) test $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-release.out -race -count=1 ./pkg/... -run TestIntegrationRelease
	@cd v2 && $(GO) test $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-additional.out -race -count=1 ./pkg/... -run TestIntegrationAdditionalM2M
	@cd v2 && $(GO) test $(GO_BUILD_FLAGS) -coverprofile=tests/results-integration/cover-release.out -race -count=1 ./pkg/... -run TestIntegrationReleaseM2M

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
#v2 workspace required change
#$(GO) fmt $(GO_MOD_FLAGS) ./pkg/...
#$(GO) fmt $(GO_MOD_FLAGS) ./cmd/...
	$(GO) fmt ./pkg/...
	$(GO) fmt ./cmd/...
.PHONY: format

vet:
#v2 workspace required change
#$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./pkg/...
#$(GO) vet $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) ./cmd/...
	$(GO) vet $(GO_BUILD_FLAGS) ./pkg/...
	$(GO) vet $(GO_BUILD_FLAGS) ./cmd/...
.PHONY: vet

#v2 workspace required change
# build: 
# 	mkdir -p $(GO_BUILD_BINDIR)
# 	go build $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) $(GO_LD_FLAGS) -o $(GO_BUILD_BINDIR) ./...
# .PHONY: build