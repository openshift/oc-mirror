# docker build -f Dockerfile -t local/go-toolset .
# docker run -it --rm --privileged -v ${PWD}:/build:z local/go-toolset
#################################################################################
# Builder Image
FROM registry.access.redhat.com/ubi8/ubi as builder

#################################################################################
# DNF Package Install List
ARG DNF_LIST="\
  ### base deps
  util-linux \
  coreutils-single \
  glibc-minimal-langpack \
  ### build deps
  gcc \
  make \
  podman \
  shadow-utils \
  fuse-overlayfs /etc/containers/storage.conf \
"

#################################################################################
# DNF Package Install Flags
ARG DNF_FLAGS="\
  -y \
  --releasever 8 \
  --installroot /rootfs \
  --exclude container-selinux \
"
ARG DNF_FLAGS_EXTRA="\
  --nodocs \
  --setopt=install_weak_deps=false \
  ${DNF_FLAGS} \
"

#################################################################################
# Build UBI8 Rootfs
ARG BUILD_PATH='/rootfs'
RUN set -ex \
     && dnf install jq tar -y \
     && mkdir -p ${BUILD_PATH}                                                  \
     && cp -r /etc/pki            /rootfs/etc/                                  \
     && cp -r /etc/yum.repos.d    /rootfs/etc/                                  \
     && cp -r /etc/os-release     /rootfs/etc/os-release                        \
     && cp -r /etc/redhat-release /rootfs/etc/redhat-release                    \
     && dnf -y module enable container-tools:rhel8                              \
     && dnf install ${DNF_FLAGS_EXTRA} ${DNF_LIST}                              \
     && dnf update ${DNF_FLAGS_EXTRA}                                           \
     && dnf install ${DNF_FLAGS_EXTRA} 'dnf-command(copr)'                      \
     && dnf ${DNF_FLAGS_EXTRA} copr enable rhcontainerbot/container-selinux     \
     && dnf ${DNF_FLAGS_EXTRA} reinstall shadow-utils                           \
     && dnf clean all ${DNF_FLAGS}                                              \
     && rm -rf                                                                  \
           ${BUILD_PATH}/var/cache/*                                            \
           ${BUILD_PATH}/var/log/dnf*                                           \
           ${BUILD_PATH}/var/log/yum*                                           \
     && du -sh ${BUILD_PATH}                                                    \
     && mkdir -p                                                                \
              ${BUILD_PATH}/root/.docker                                        \
              ${BUILD_PATH}/var/lib/shared/overlay-images                       \
              ${BUILD_PATH}/var/lib/shared/overlay-layers                       \
     && echo '{}' > ${BUILD_PATH}/root/.docker/config.json                      \
     && touch ${BUILD_PATH}/var/lib/shared/overlay-images/images.lock           \
     && touch ${BUILD_PATH}/var/lib/shared/overlay-layers/layers.lock           \
     && sed -i                                                                  \
               -e 's|^#mount_program|mount_program|g'                           \
               -e '/additionalimage.*/a "/var/lib/shared",'                     \
            ${BUILD_PATH}/etc/containers/storage.conf                           \
     && sed -i                                                                  \
               -e '/^cgroup_manager.*/d'                                        \
               -e '/\#\ cgroup_manager\ =/a cgroup_manager = "cgroupfs"'        \
            ${BUILD_PATH}/usr/share/containers/containers.conf                  \
     && sed -i 's/10.88.0/10.89.0/g' ${BUILD_PATH}/etc/cni/net.d/87-podman-bridge.conflist \
     && sed -i                                                                  \
               -e 's|"/var/lib/shared",|#"/var/lib/shared",|'                   \
            ${BUILD_PATH}/etc/containers/storage.conf                           \
      && export GO_VERSION="$( curl https://golang.org/dl/?mode=json -s         \
                             | jq -r '.[].files[].version'                      \
                             | grep -v -E 'go[0-9\.]+(beta|rc)'                 \
                             | sort | tail -1)"                                 \
      && curl -L https://golang.org/dl/${GO_VERSION}.linux-amd64.tar.gz         \
         | tar xzvf - --directory ${BUILD_PATH}/usr/local/                      \
      && ${BUILD_PATH}/usr/local/go/bin/go version                              \
    && echo

#################################################################################
# Create image from rootfs
FROM scratch
COPY --from=builder /rootfs /
ADD ./rootfs /
RUN set -x \
    && ln -f /usr/local/go/bin/go /usr/bin/go                                   \
    && go version                                                               \
    && echo

#################################################################################
# Finalize image
WORKDIR /build
ENTRYPOINT ["/entrypoint.sh"]
CMD ""

ENV \
  STORAGE_DRIVER=overlay \
  BUILDAH_ISOLATION=chroot \
  _BUILDAH_STARTED_IN_USERNS="" \
  REGISTRY_AUTH_FILE='/root/.docker/config.json' \
  PATH="/root/platform/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

LABEL \
  name="go-toolset"                                                             \
  license=GPLv3                                                                 \
  distribution-scope="public"                                                   \
  io.openshift.tags="go-toolset"                                                \
  summary="Bundle compiler image"                                               \
  io.k8s.display-name="go-toolset"                                              \
  build_date="`date +'%Y%m%d%H%M%S'`"                                           \
  project="https://github.com/RedHatGov/bundle"                                 \
  description="Bundle is designed to automate delarative enterprise artifact supply chain."\
  io.k8s.description="Bundle is designed to automate delarative enterprise artifact supply chain."
