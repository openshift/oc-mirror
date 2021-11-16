FROM docker.io/library/golang:1.17.1-bullseye
RUN apt-get update -y && apt install build-essential pigz jq skopeo -y
ADD https://github.com/mikefarah/yq/releases/download/v4.13.5/yq_linux_amd64 /usr/bin/yq
RUN chmod +x /usr/bin/yq