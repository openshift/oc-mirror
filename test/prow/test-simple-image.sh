#Simple additional image test 
set -e
mkdir -p /tmp/test-simple-image/archives

./bin/oc-mirror --source-skip-tls --dest-skip-tls --skip-cleanup --config test/prow/configs/imageset-image.yaml --dir /tmp/test-simple-image file:///tmp/test-simple-image/archives
./bin/oc-mirror --source-skip-tls --dest-skip-tls --skip-cleanup --from "/tmp/test-simple-image/archives/mirror_seq1_000000.tar" --dir "/tmp/test-simple-image" docker://localhost:5000

local=$(skopeo inspect docker://localhost:5000/fedora:latest --tls-verify=false | jq '.Digest')
remote=$(skopeo inspect docker://registry.fedoraproject.org/fedora:latest | jq '.Digest')

if [ $local = $remote ]
  then
    echo "Digest match"
  else
    echo "Digest does not match against additional image ref"
    exit 1
fi