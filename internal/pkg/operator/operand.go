package operator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// OperandResolver reads CSV controller images only after a platform mismatch.
// Cache successful reads; serialize this rare fallback to avoid duplicate pulls.
type OperandResolver struct {
	Opts    *mirror.CopyOptions
	handler bundleHandlerInterface
	mu      sync.Mutex
	bundles map[string]map[string]struct{}
}

func NewOperandResolver(log clog.PluggableLoggerInterface, opts *mirror.CopyOptions, copier mirror.MirrorInterface, manifests manifest.ManifestInterface) *OperandResolver {
	return &OperandResolver{
		Opts:    opts,
		handler: BundleHandler{Log: log, Mirror: copier, Manifest: manifests},
	}
}

// IsOperand never skips an image used by any listing bundle's deployments.
func (r *OperandResolver) IsOperand(ctx context.Context, origin string, bundles map[string]string) (bool, error) {
	if len(bundles) == 0 {
		return false, nil
	}
	ref := referenceWithoutTransport(origin)
	candidate, parseErr := image.ParseRef(ref)
	for bundle := range bundles {
		controllers, err := r.controllersOf(ctx, bundle)
		if err != nil {
			return false, err
		}
		if _, runsIt := controllers[ref]; runsIt {
			return false, nil
		}
		if parseErr != nil {
			continue
		}
		for controller := range controllers {
			if spec, err := image.ParseRef(controller); err == nil && sameImage(candidate, spec) {
				return false, nil
			}
		}
	}
	return true, nil
}

// Tags and digests cannot be compared without resolving the tag. Conservatively
// consider them a match, so an operator's controller cannot be silently skipped.
func sameImage(a, b image.ImageSpec) bool {
	if a.Name != b.Name {
		return false
	}
	switch {
	case a.Digest != "" && b.Digest != "":
		return a.Digest == b.Digest
	case a.Tag != "" && b.Tag != "":
		return a.Tag == b.Tag
	default:
		return true
	}
}

func (r *OperandResolver) controllersOf(ctx context.Context, bundle string) (map[string]struct{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if images, ok := r.bundles[bundle]; ok {
		return images, nil
	}
	dir := filepath.Join(r.Opts.Global.WorkingDir, operatorBundlesDir, fmt.Sprintf("%x", sha256.Sum256([]byte(bundle))))
	images, err := r.handler.ControllerImages(ctx, bundle, dir, *r.Opts)
	if err == nil {
		if r.bundles == nil {
			r.bundles = make(map[string]map[string]struct{})
		}
		r.bundles[bundle] = images
	}
	return images, err
}

func referenceWithoutTransport(ref string) string {
	if _, after, found := strings.Cut(ref, "://"); found {
		return after
	}
	return ref
}
