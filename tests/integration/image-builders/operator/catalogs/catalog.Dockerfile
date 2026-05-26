ARG CATALOG="catalog"

FROM quay.io/operator-framework/opm:latest

COPY ${CATALOG} /configs

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# DC-specific label for the location of the DC root directory in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
