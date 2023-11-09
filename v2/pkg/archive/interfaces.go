package archive

type BlobsGatherer interface {
	GatherBlobs(imgRef string) (map[string]string, error)
}
