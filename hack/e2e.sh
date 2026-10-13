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

build_e2e_image () {
  run_log 0 "Starting builder container image build"
  ${run_cmd} build -f Dockerfile -t ${image_name} .
}

run_e2e () {
  run_log 0 "Starting binary build"
  ${run_cmd} run -it -v ${src_dir}:/build:z ${image_name} "test-e2e"
}

run () {
  build_e2e_image \
    && run_log 0 "Successfully built e2e image" \
    || run_log 1 "Failed to build e2e image"
  run_e2e \
    && run_log 0 "Successfully ran e2e" \
    || run_log 1 "Failed e2e"
}

run
