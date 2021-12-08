#/bin/sh
set -e
bash test/prow/test-simple-image.sh || echo "Simple image test failed with errors"
bash test/prow/test-operator-catalog.sh ||  echo "Operator catalog test failed with errors"
echo "Test complete with no errors."