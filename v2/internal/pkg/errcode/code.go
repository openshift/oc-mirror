package errcode

const (
	GenericErr       = 1 << 0
	ReleaseErr       = 1 << 1
	OperatorErr      = 1 << 2
	AdditionalImgErr = 1 << 3
	HelmErr          = 1 << 4
)
