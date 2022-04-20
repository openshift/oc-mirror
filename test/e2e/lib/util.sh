#!/usr/bin/env bash

# run_cmd runs $CMD <args> <test flags> where test flags are arguments
# needed to run against a local test registry and provide informative
# debug data in case of test errors.
function run_cmd() {
  local test_flags="--log-level debug --dest-use-http --skip-cleanup"

  echo "$CMD" "$@" $test_flags
  echo
  "$CMD" "$@" $test_flags
}

# cleanup_all will kill any running registry processes
function cleanup_all() {
    [[ -n $PID_DISCONN ]] && kill $PID_DISCONN
    [[ -n $PID_CONN ]] && kill $PID_CONN
}

# cleanup_conn will only kill the connected registry
function cleanup_conn() {
    [[ -n $PID_CONN ]] && kill $PID_CONN
}

# install_deps will install crane and registry2 in go bin dir
function install_deps() {
  pushd ${DATA_TMP}
  GOFLAGS=-mod=mod go install github.com/google/go-containerregistry/cmd/crane@latest
  popd
  crane export registry:2 registry2.tar
  tar xvf registry2.tar bin/registry
  mv bin/registry $GOBIN
  crane export quay.io/operator-framework/opm@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19 opm.tar
  tar xvf opm.tar bin/opm
  mv bin/opm $GOBIN
  rm -f registry2.tar opm.tar
}

# setup_reg will configure and start registry2 processes
# for connected and disconnected environments
function setup_reg() {
  # Setup connected registry
  echo -e "Setting up registries"
  cp ${DIR}/e2e-config.yaml ${DATA_TMP}/conn.yaml
  find "${DATA_TMP}" -type f -exec sed -i -E 's@TMP@'"${REGISTRY_CONN_DIR}"'@g' {} \;
  find "${DATA_TMP}" -type f -exec sed -i -E 's@PORT@'"${REGISTRY_CONN_PORT}"'@g' {} \;
  DPORT=$(expr ${REGISTRY_CONN_PORT} + 10)
  find "${DATA_TMP}" -type f -exec sed -i -E 's@DEBUG@'"$DPORT"'@g' {} \;
  registry serve ${DATA_TMP}/conn.yaml &> ${DATA_TMP}/coutput.log &
  PID_CONN=$!
  # Setup disconnected registry
  cp ${DIR}/e2e-config.yaml ${DATA_TMP}/disconn.yaml
  find "${DATA_TMP}" -type f -exec sed -i -E 's@TMP@'"${REGISTRY_DISCONN_DIR}"'@g' {} \;
  find "${DATA_TMP}" -type f -exec sed -i -E 's@PORT@'"${REGISTRY_DISCONN_PORT}"'@g' {} \;
  DPORT=$(expr $REGISTRY_DISCONN_PORT + 10)
  find "${DATA_TMP}" -type f -exec sed -i -E 's@DEBUG@'"${DPORT}"'@g' {} \;
  registry serve ${DATA_TMP}/disconn.yaml &> ${DATA_TMP}/doutput.log &
  PID_DISCONN=$!

  echo -e "disconnected registry PID: $PID_DISCONN"
  echo -e "connected registry PID: $PID_CONN"
}

# prep_registry will copy the needed catalog image
# to the connected registry
function prep_registry() {
   local CATALOGTAG="${1:?CATALOGTAG required}"
  # Copy target catalog to connected registry
    crane copy --insecure ${CATALOGREGISTRY}/${CATALOGNAMESPACE}:${CATALOGTAG} \
    localhost.localdomain:${REGISTRY_CONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest

    CATALOGDIGEST=$(crane digest --insecure localhost.localdomain:${REGISTRY_CONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest)
}



# parse_args will parse common arguments
# for each workflow function
function parse_args() {
  # Set default values
  NS=""
  CREATE_FLAGS=""
  PUBLISH_FLAGS=""
  DIFF=false
  for i in "$@"; do
  case "$i" in
    -n=*|--ns=*)
      NS="/${i#*=}"
      shift # past argument=value
      ;;
    -c=*|--create_flags=*)
      CREATE_FLAGS="${i#*=}"
      shift # past argument=value
      ;;
    -p=*|--publish_flags=*)
      PUBLISH_FLAGS="${i#*=}"
      shift # past argument=value
      ;;
    --diff)
      DIFF=YES
      shift # past argument with no value
      ;;
    -*|--*)
      echo "Unknown option $i"
      exit 1
      ;;
    *)
      ;;
  esac
done
}

# setup_operator_testdata will move required
# files in place to do operator testing
function setup_operator_testdata() {
  local DATA_DIR="${1:?DATA_DIR required}"
  local OUTPUT_DIR="${2:?OUTPUT_DIR required}"
  local CONFIG_PATH="${3:?CONFIG_PATH required}"
  local DIFF="${4:?DIFF bool required}"
  local CATALOG_DIGEST="${5:-""}"
  if $DIFF; then
    INDEX_PATH=diff
  else
    INDEX_PATH=latest
  fi
  echo -e "\nSetting up test directory in $DATA_DIR"
  mkdir -p "$OUTPUT_DIR"
  cp "${DIR}/configs/${CONFIG_PATH}" "${OUTPUT_DIR}/"
  find "$DATA_DIR" -type f -exec sed -i -E 's@METADATA_CATALOGNAMESPACE@'"$METADATA_CATALOGNAMESPACE"'@g' {} \;
  find "$DATA_DIR" -type f -exec sed -i -E 's@CATALOG_DIGEST@'"$CATALOG_DIGEST"'@g' {} \;
  find "$DATA_DIR" -type f -exec sed -i -E 's@DATA_TMP@'"$DATA_DIR"'@g' {} \;
}

# setup_helm_testdata will move required
# files in place to do helm testing
function setup_helm_testdata() {
  local DATA_DIR="${1:?DATA_DIR required}"
  local OUTPUT_DIR="${2:?OUTPUT_DIR required}"
  local CONFIG_PATH="${3:?CONFIG_PATH required}"
  local CHART_PATH="${4:?CHART_PATH is required}"
  echo -e "\nSetting up test directory in $DATA_DIR"
  mkdir -p "$OUTPUT_DIR"
  cp "${DIR}/configs/${CONFIG_PATH}" "${OUTPUT_DIR}/"
  cp "${DIR}/artifacts/${CHART_PATH}" "${DATA_DIR}/"
  find "$DATA_DIR" -type f -exec sed -i -E 's@DATA_TMP@'"$DATA_DIR"'@g' {} \;
}