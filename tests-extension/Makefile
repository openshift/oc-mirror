BINARY := bin/oc-mirror-tests-ext

.PHONY: vendor
vendor:
	GOTOOLCHAIN=auto go mod vendor

.PHONY: build
build: vendor
	@echo "Building extension binary..."
	@cd test/e2e && $(MAKE) -f bindata.mk update-bindata
	@mkdir -p bin
	GOTOOLCHAIN=auto go build -mod=vendor -o $(BINARY) ./cmd
	@echo "Binary built: $(BINARY)"

.PHONY: clean
clean:
	@rm -f $(BINARY)
	@cd test/e2e && $(MAKE) -f bindata.mk clean-bindata

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build  - Build extension binary"
	@echo "  clean  - Remove binaries and bindata"