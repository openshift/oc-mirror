REGISTRY ?= quay.io/oc-mirror/oc-mirror-dev
RUNTIME ?= podman
OUTPUT = _output

all: build
.PHONY: all

build:
	go run ./cmd/build-catalogs
.PHONY: build

push: push-bundles push-related push-catalogs
.PHONY: push

push-bundles:
	@for dir in bundles/*/*-bundle-v*/; do \
		tag=$$(basename "$$dir"); \
		echo "Pushing bundle $$tag"; \
		$(RUNTIME) build --format=docker -t "$(REGISTRY):$$tag" "$$dir"; \
		$(RUNTIME) push --format=docker "$(REGISTRY):$$tag"; \
	done
.PHONY: push-bundles

push-related: push-bundles
	@for dir in bundles/*/*-bundle-v*/; do \
		bundle_tag=$$(basename "$$dir"); \
		tag=$$(echo "$$bundle_tag" | sed 's/-bundle//'); \
		echo "Pushing related image $$tag"; \
		$(RUNTIME) build --no-cache --format=docker \
			--build-arg RELATED_IMAGE="$(REGISTRY):$$bundle_tag" \
			-t "$(REGISTRY):$$tag" related_image/; \
		$(RUNTIME) push --format=docker "$(REGISTRY):$$tag"; \
	done
.PHONY: push-related

push-catalogs: build
	@for dir in $(OUTPUT)/test-catalog-*/; do \
		name=$$(basename "$$dir"); \
		echo "Pushing catalog $$name"; \
		$(RUNTIME) build --format=docker -t "$(REGISTRY):$$name" \
			-f "$(OUTPUT)/$$name.Dockerfile" $(OUTPUT); \
		$(RUNTIME) push --format=docker "$(REGISTRY):$$name"; \
	done
.PHONY: push-catalogs

clean:
	rm -rf $(OUTPUT)
.PHONY: clean
