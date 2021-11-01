#!/bin/bash

image_name="local/go-toolset"
run_cmd=$(command -pv podman || command -pv docker)
src_dir="$(pwd)"

run_log () {
  if [[ $1 == 0 ]]; then
    echo ">>  INFO: $2"
  elif [[ $1 != 0 ]]; then
    echo ">>  ERROR: $2"
    exit 1
  fi
}

build_builder_image () {
  run_log 0 "Starting builder container image build"
  ${run_cmd} build -f Dockerfile -t ${image_name} .
}

build_binary () {
  run_log 0 "Starting binary build"
  ${run_cmd} run -it --rm --privileged -v ${src_dir}:/build:z ${image_name}
}

run () {
  build_builder_image \
    && run_log 0 "Successfully built builder image" \
    || run_log 1 "Failed to build builder image"
  build_binary \
    && run_log 0 "Successfully built binary" \
    || run_log 1 "Failed to build binary"
}

run
