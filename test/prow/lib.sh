function check_bundles() {
  #podman manifest inspect localhost:5000/test-catalogs/test-catalog:latest
  #Error: error inspect manifest localhost:5000/test-catalogs/test-catalog:latest: loading manifest "localhost:5000/test-catalogs/test-catalog:latest": unable to load manifest list: unsupported format "application/vnd.docker.distribution.manifest.v2+json": manifest type not supported
  #TODO
  #Possible alternatives...
  #sudo skopeo inspect docker://localhost:5000/test-catalogs/test-catalog:latest --tls-verify=false
}