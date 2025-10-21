package archive

const (
	archiveFilePrefix           = "mirror"
	imageSetConfigPrefix        = "isc_"
	cacheRepositoriesDir        = "docker/registry/v2/repositories"
	cacheBlobsDir               = "docker/registry/v2/blobs"
	cacheFilePrefix             = "docker/registry/v2"
	workingDirectory            = "working-dir"
	errMessageFolder            = "unable to create folder %s: %v"
	segMultiplier         int64 = 1024 * 1024 * 1024
	defaultSegSize        int64 = 500
	archiveFileNameFormat       = "%s_%06d.tar"
)
