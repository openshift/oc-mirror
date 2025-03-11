package common

const (
	TestFolder string = "../../../tests/"

	GenericErrCode       = 1 << 0
	ReleaseErrCode       = 1 << 1
	OperatorErrCode      = 1 << 2
	AdditionalImgErrCode = 1 << 3
	HelmErrCode          = 1 << 4
)
