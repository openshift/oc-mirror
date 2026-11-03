package archive

const (
	archiveFilePrefix           = "mirror"
	imageSetConfigPrefix        = "isc_"
	cacheRepositoriesDir        = "docker/registry/v2/repositories"
	cacheBlobsDir               = "docker/registry/v2/blobs"
	cacheFilePrefix             = "docker/registry/v2"
	workingDirectory            = "working-dir"
	segMultiplier         int64 = 1024 * 1024 * 1024
	defaultSegSize        int64 = 500
	archiveFileNameFormat       = "%s_%06d.tar"
	// MirrorArchiveFileRegexp matches producer archive names (mirror_NNNNNN.tar).
	// Anchored so suffixes like .tar.backup are not treated as archives.
	MirrorArchiveFileRegexp = `^` + archiveFilePrefix + `_[0-9]{6}\.tar$`
)
