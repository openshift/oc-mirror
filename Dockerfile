# docker build -f Dockerfile -t local/go-toolset .
# docker run -it --rm --privileged -v ${PWD}:/build:z local/go-toolset
#################################################################################
# Builder Image
FROM registry.access.redhat.com/ubi9/ubi

#################################################################################
# DNF Package Install List
ARG DNF_LIST="\
  jq \
  tar \
  gcc \
  make \
  git \
  gpgme-devel \
  libassuan-devel \
  wget \
  pigz \
  procps-ng \
"

#################################################################################
# Build UBI8 Builder with multi-arch support
RUN set -ex \
     && ARCH=$(arch | sed 's|x86_64|amd64|g' | sed 's|aarch64|arm64|g')         \
     && dnf install -y --nodocs --setopt=install_weak_deps=false ${DNF_LIST}    \
     && dnf clean all -y                                                        \
     && GO_VERSION=go1.23.5                                                     \
     && curl -sL https://golang.org/dl/${GO_VERSION}.linux-${ARCH}.tar.gz       \
        | tar xzvf - --directory /usr/local/                                    \
     && /usr/local/go/bin/go version                                            \
     && ln -f /usr/local/go/bin/go /usr/bin/go

#################################################################################
# Link gcc to /usr/bin/s390x-linux-gnu-gcc as go requires it on s390x
RUN [ "$(arch)" == "s390x" ]                                                    \
     && ln /usr/bin/gcc /usr/bin/s390x-linux-gnu-gcc                            \
     || echo "Not running on s390x, skip linking gcc binary"

WORKDIR /build
ENTRYPOINT ["make"]
CMD []

ENV PATH="/root/platform/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
ENV GOROOT="/usr/local/go"

LABEL \
  name="go-toolset"                                                             \
  license=GPLv3                                                                 \
  distribution-scope="public"                                                   \
  io.openshift.tags="go-toolset"                                                \
  summary="oc-mirror compiler image"                                            \
  io.k8s.display-name="go-toolset"                                              \
  build_date="`date +'%Y%m%d%H%M%S'`"                                           \
  project="https://github.com/openshift/oc-mirror"                              \
  description="oc-mirror is an OpenShift Client (oc) plugin that manages OpenShift release, operator catalog, helm charts, and associated container images. This image is designed to build the binary." \
  io.k8s.description="oc-mirror is an OpenShift Client (oc) plugin that manages OpenShift release, operator catalog, helm charts, and associated container images. This image is designed to build the binary."
