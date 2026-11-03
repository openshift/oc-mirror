REGISTRY ?= quay.io/oc-mirror/oc-mirror-dev
IMAGE_VERSION ?= $(shell date -u +%Y%m%d)-$(shell git rev-parse --short HEAD)
TAG ?= multilayers-$(IMAGE_VERSION)
RUNTIME ?= podman

IMAGE_REF = $(REGISTRY):$(TAG)

.PHONY: all build push digest

all: build

build:
	$(RUNTIME) build -f Dockerfile -t $(IMAGE_REF) .

# Maintainer-only: run manually after build.
push: build
	$(RUNTIME) push $(IMAGE_REF)

digest:
	@$(RUNTIME) inspect $(IMAGE_REF) --format '{{index .Digest}}'
