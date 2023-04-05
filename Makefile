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
	$(GO) vet $(GO_BUILD_FLAGS) ./pkg/... 
	$(GO) vet $(GO_BUILD_FLAGS) ./cmd/...  
.PHONY: vet