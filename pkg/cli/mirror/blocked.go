package mirror

import (
	"fmt"
	"regexp"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
)

type ErrBlocked struct {
	image string
}

func (e ErrBlocked) Error() string {
	return fmt.Sprintf("image %s blocked", e.image)
}

// IsBlocked will return a boolean value on whether an image
// is specified as blocked in the ImageSetConfigSpec
func isBlocked(blocked []v1alpha2.Image, imgRef string) (bool, error) {
	for _, img := range blocked {
		matcher, err := regexp.Compile(img.Name)
		if err != nil {
			return false, fmt.Errorf("error parsing blocked image regular expression %s: %v", img.Name, err)
		}

		if matcher.MatchString(imgRef) {
			return true, nil
		}
	}
	return false, nil
}
