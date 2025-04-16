package archive

import (
	"fmt"
)

type SignatureBlobGathererError struct {
	SigError error
}

func (e SignatureBlobGathererError) Error() string {
	return fmt.Sprintf("signature error: %s", e.SigError)
}
