package v2alpha1

// BlobSizeInfo represents a single blob's digest and its compressed size in bytes.
type BlobSizeInfo struct {
	Digest string
	Size   int64
}

// ImageSizeResult holds the sizing result for one image.
type ImageSizeResult struct {
	Source string
	Blobs  []BlobSizeInfo
	Error  string
}

// SizeReportError records one image whose manifest could not be fetched for sizing.
type SizeReportError struct {
	Image string
	Error string
}

// DryRunSizeReport is the aggregate size report computed during --dry-run and
// logged as a human-readable summary.
type DryRunSizeReport struct {
	TotalSize       int64
	ImageCount      int
	Errors          []SizeReportError
	SizeUnavailable bool // true when the total could not be determined (e.g. unreadable archive)
}
