# docker build -f Dockerfile -t local/go-toolset .
# docker run -it --rm --privileged -v ${PWD}:/build:z local/go-toolset
#################################################################################
# Builder Image
FROM registry.access.redhat.com/ubi8/ubi

#################################################################################
# DNF Package Install List
ARG DNF_LIST="\
  jq \
  tar \
  gcc \
  make \
  git \
"

#################################################################################
# Build UBI8 Builder
RUN set -ex \
     && dnf install -y --nodocs --setopt=install_weak_deps=false ${DNF_LIST}    \
     && dnf clean all -y                                                        \
     && GO_VERSION=$(curl -sL https://golang.org/dl/?mode=json                  \
                     | jq -r '.[].files[] | select(.os == "linux").version'     \
                     | grep -v -E 'go[0-9\.]+(beta|rc)'                         \
                     | sort -V | tail -1)                                       \
     && curl -sL https://golang.org/dl/${GO_VERSION}.linux-amd64.tar.gz         \
        | tar xzvf - --directory /usr/local/                                    \
     && /usr/local/go/bin/go version                                            \
     && ln -f /usr/local/go/bin/go /usr/bin/go

WORKDIR /build
ENTRYPOINT ["make"]
CMD []

ENV PATH="/root/platform/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

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
