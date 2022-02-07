package image

type ImageType int

const (
	TypeInvalid ImageType = iota
	TypeOCPRelease
	TypeOperatorCatalog
	TypeOperatorBundle
	TypeOperatorRelatedImage
	TypeGeneric
)

var imageTypeStrings = map[ImageType]string{
	TypeOCPRelease:           "ocpRelease",
	TypeOperatorCatalog:      "operatorCatalog",
	TypeOperatorBundle:       "operatorBundle",
	TypeOperatorRelatedImage: "operatorRelatedImage",
	TypeGeneric:              "generic",
}

func (it ImageType) String() string {
	return imageTypeStrings[it]
}
