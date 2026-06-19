REGISTRY ?= quay.io/oc-mirror/oc-mirror-dev
RUNTIME ?= podman
OUTPUT = _output
COSIGN_KEY ?= ../../testdata/keys/cosign.key

CATALOGDIRS := $(foreach DIR,$(shell find catalogs -maxdepth 1 -mindepth 1 -type d),$(subst catalogs/,,$(DIR)))
CATALOG_TARGETS := $(addprefix build-catalog.,$(CATALOGDIRS))
CATALOG_PUSH_TARGETS := $(addprefix push-catalog.,$(CATALOGDIRS))

all: build
.PHONY: all

build: $(CATALOG_TARGETS)
$(CATALOG_TARGETS): build-catalog.%:
	@echo "Building catalog $*"
	cd "catalogs" && $(RUNTIME) build --format=docker -f "./catalog.Dockerfile" --build-arg "CATALOG=$*" -t "$(REGISTRY):$*" .

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

push-catalogs: $(CATALOG_PUSH_TARGETS)
$(CATALOG_PUSH_TARGETS): push-catalog.%:
	@echo "Pushing catalog $*"
	$(RUNTIME) push --format=docker "$(REGISTRY):$*"
.PHONY: push-catalogs

sign: sign-bundles sign-related sign-catalogs
.PHONY: sign

sign-bundles:
	@for dir in bundles/*/*-bundle-v*/; do \
		tag=$$(basename "$$dir"); \
		echo "Signing bundle $$tag"; \
		COSIGN_PASSWORD="" cosign sign --key $(COSIGN_KEY) --tlog-upload=false "$(REGISTRY):$$tag"; \
	done
.PHONY: sign-bundles

sign-related:
	@for dir in bundles/*/*-bundle-v*/; do \
		bundle_tag=$$(basename "$$dir"); \
		tag=$$(echo "$$bundle_tag" | sed 's/-bundle//'); \
		echo "Signing related image $$tag"; \
		COSIGN_PASSWORD="" cosign sign --key $(COSIGN_KEY) --tlog-upload=false "$(REGISTRY):$$tag"; \
	done
.PHONY: sign-related

sign-catalogs: $(addprefix sign-catalog.,$(CATALOGDIRS))
$(addprefix sign-catalog.,$(CATALOGDIRS)): sign-catalog.%:
	@echo "Signing catalog $*"
	COSIGN_PASSWORD="" cosign sign --key $(COSIGN_KEY) --tlog-upload=false "$(REGISTRY):$*"
.PHONY: sign-catalogs

clean:
	rm -rf $(OUTPUT)
.PHONY: clean
