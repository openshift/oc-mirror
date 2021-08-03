#!/usr/bin/env bash

echo -e "\nStarting containerized docker registry"

docker run -d -p 5000:5000 --restart always --name registry registry:2
