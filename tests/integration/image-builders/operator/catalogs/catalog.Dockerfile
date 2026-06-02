FROM quay.io/operator-framework/opm:latest as builder
ARG CATALOG

# Copy FBC root into image at /configs and pre-populate serve cache
ADD $CATALOG /configs/
RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

FROM quay.io/operator-framework/opm:latest

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

COPY --from=builder /configs /configs
COPY --from=builder /tmp/cache /tmp/cache

# FBC-specific label for the location of the FBC root directory in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
