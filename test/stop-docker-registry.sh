#!/usr/bin/env bash

echo -e "\nCleaning up docker registry"

docker stop registry || true
docker rm registry || true
