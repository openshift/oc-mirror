FROM docker.io/library/golang:1.17.1-bullseye
ADD https://github.com/moby/buildkit/releases/download/v0.9.1/buildkit-v0.9.1.linux-amd64.tar.gz .
RUN tar -xvf buildkit-v0.9.1.linux-amd64.tar.gz && chmod +x bin/* && mv bin/* /usr/bin && rm buildkit-v0.9.1.linux-amd64.tar.gz
RUN apt-get update -y && apt install build-essential pigz -y
ADD https://github.com/mikefarah/yq/releases/download/v4.13.5/yq_linux_amd64 /usr/bin/yq
RUN chmod +x /usr/bin/yq
#This will be refined with a prow job pod mutation or similar (though may not be neccessary with the openshift internal registry...but that's only 1 registry)
#Also needs to be added to buildkitd daemon.
COPY registry-cert.crt /usr/local/share/ca-certificates/registry-cert.crt
RUN chmod 644 /usr/local/share/ca-certificates/registry-cert.crt && update-ca-certificates
RUN apt-get install podman -y

#buildctl --addr kube-pod://buildkitd  build --frontend dockerfile.v0 --local context=. --local dockerfile=. --output type=image,name=target.registry.net/library/builder,push=true