GO := go

ifdef OS_GIT_VERSION
SOURCE_GIT_TAG := ${OS_GIT_VERSION}
endif

GOMODCACHE=$(shell $(GO) env GOMODCACHE)

BUILD_MACHINERY_VERSION=v0.0.0-20240910153727-5725581bdf8f
BUILD_MACHINERY_PATH=github.com/openshift/build-machinery-go

$(shell $(GO) get ${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION})

# Include the library makefile
include $(addprefix ${GOMODCACHE}/${BUILD_MACHINERY_PATH}@${BUILD_MACHINERY_VERSION}/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps-gomod.mk \
)

GO_LD_EXTRAFLAGS=$(call version-ldflags,github.com/openshift/oc-mirror/v2/internal/pkg/version)

.PHONY: all test build clean

REGISTRY_BASE ?= quay.io/luzuccar
IMAGE_NAME ?= golang-oci-mirror
IMAGE_VERSION ?= v0.0.1

GO_BUILD_BINDIR := ./build
GO_MOD_FLAGS = -mod=readonly
GO_BUILD_PACKAGES := ./cmd/oc-mirror
GO_PACKAGE = github.com/openshift/oc-mirror/v2

LIBDM_BUILD_TAG = $(shell ../hack/libdm_tag.sh)
LIBSUBID_BUILD_TAG = $(shell ../hack/libsubid_tag.sh)
BTRFS_BUILD_TAG = $(shell ../hack/btrfs_tag.sh) $(shell ../hack/btrfs_installed_tag.sh)

ifeq ($(DISABLE_CGO), 1)
	override BTRFS_BUILD_TAG = exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp
endif

GO_BUILD_FLAGS = -tags "json1 $(BTRFS_BUILD_TAG) $(LIBDM_BUILD_TAG) $(LIBSUBID_BUILD_TAG)"

all: clean test build

build: 
	mkdir -p $(GO_BUILD_BINDIR)
	go build $(GO_MOD_FLAGS) $(GO_BUILD_FLAGS) $(GO_LD_FLAGS) -race -o $(GO_BUILD_BINDIR) ./...
	

build-dev:
	mkdir -p build
	GOOS=linux go build -ldflags="-s -w" -o build -tags real./...
	chmod 755 build/microservice
	chmod 755 build/uid_entrypoint.sh

verify:
	golangci-lint run -c .golangci.yaml --deadline=30m

test:
	mkdir -p tests/results
	go test -v -short -coverprofile=tests/results/cover.out ./...

test-integration:
	mkdir -p tests/results-integration
	go test -coverprofile=tests/results-integration/cover-additional.out  -race -count=1 ./... -run TestIntegrationAdditional
	go test -coverprofile=tests/results-integration/cover-release.out  -race -count=1 ./... -run TestIntegrationRelease
	go test -coverprofile=tests/results-integration/cover-additional.out  -race -count=1 ./... -run TestIntegrationAdditionalM2M
	go test -coverprofile=tests/results-integration/cover-release.out  -race -count=1 ./... -run TestIntegrationReleaseM2M


cover:
	go tool cover -html=tests/results/cover.out -o tests/results/cover.html

clean:
	rm -rf build/*
	go clean ./...

container:
	podman build -t  ${REGISTRY_BASE}/${IMAGE_NAME}:${IMAGE_VERSION} .

push:
	podman push --authfile=${HOME}/.docker/config.json ${REGISTRY_BASE}/${IMAGE_NAME}:${IMAGE_VERSION} 
