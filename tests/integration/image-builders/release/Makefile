.PHONY: all build clean

COSIGN_KEY ?= ../../testdata/keys/cosign.key

all: clean build

build:
	mkdir -p bin
	go build -a -gcflags=all="-l -B -wb=false" -ldflags="-w -s" -o bin ./...

clean:
	rm -rf build/*
	go clean ./...

container:
	podman build -t  quay.io/oc-mirror/release/test-image:v0.0.1 -f containerfile-rhel9

digest:
	podman inspect quay.io/oc-mirror/release/test-image:v0.0.1 | jq '.[].Digest'

push:
	podman push quay.io/oc-mirror/release/test-image:v0.0.1 --authfile=$(AUTHFILE)

sign:
	COSIGN_PASSWORD="" cosign sign --key $(COSIGN_KEY) --tlog-upload=false quay.io/oc-mirror/release/test-image:v0.0.1
	COSIGN_PASSWORD="" cosign sign --key $(COSIGN_KEY) --tlog-upload=false quay.io/oc-mirror/release/test-release-index:v0.0.1
.PHONY: sign

