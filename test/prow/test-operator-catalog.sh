source lib.sh
mkdir /tmp/test-operator-catalog
mkdir /tmp/test-operator-catalog/archives
./bin/oc-mirror --log-level debug --source-skip-tls --dest-skip-tls --skip-cleanup --dir /tmp/test-operator-catalog --config test/prow/configs/imageset-operator.yaml "file:///tmp/test-operator-catalog/archives"
./bin/oc-mirror --log-level debug --source-skip-tls --dest-skip-tls --skip-cleanup --dir "/tmp/test-operator-catalog" --from "/tmp/test-operator-catalog/archives/mirror_seq1_000000.tar" "docker://localhost:5000"
check_bundles localhost:5000/test-catalogs/test-catalog:latest \
  "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
  localhost:5000
