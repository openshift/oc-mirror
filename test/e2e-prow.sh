#/bin/sh
set -e
bash prow/test-simple-image.sh
bash prow/test-operator-catalog.sh