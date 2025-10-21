FROM quay.io/operator-framework/opm:latest

# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

COPY {{ index.name }} /configs

LABEL operators.operatorframework.io.index.configs.v1=/configs
