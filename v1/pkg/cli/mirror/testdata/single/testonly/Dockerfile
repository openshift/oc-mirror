# minimize size by using scratch in builder and final image
FROM scratch as builder

# Create fake /bin/opm and /bin/grpc_health_probe
# Because these are fake you can never use this as a real catalog
# and there is no way to pre-populate the opm serve cache 
COPY opm grpc_health_probe /bin/

# Copy declarative config root into image at /configs
COPY configs /configs

FROM scratch
# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

# Use a single copy from the builder to get one layer
COPY --from=builder / /

# Set DC-specific label for the location of the DC root directory
# in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
