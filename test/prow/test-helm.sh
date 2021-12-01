pushd test/prow/
echo "Helm test init."
#oc-mirror --config imageset-operator.yaml --dir /tmp/test-operator-catalog/test-create file://tmp/test-operator-catalog/archives
#oc-mirror --from /tmp/test-operator-catalog/archives --dir /tmp/test-operator-catalog/test-publish docker://localhost:5000

popd