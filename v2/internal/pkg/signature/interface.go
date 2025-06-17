package signature

import (
	"context"
)

type SignatureInterface interface {
	GetSignatureTag(ctx context.Context, imgRef string) ([]string, error)
}
