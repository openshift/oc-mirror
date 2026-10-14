package operator

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// OperandResolver answers whether an image listed by an operator bundle is an
// operand -- workload the operator deploys -- rather than an image the operator
// runs itself.
//
// The answer only exists in the bundle's ClusterServiceVersion: an operand
// image carries nothing marking it as one, and the catalog's olm.csv.metadata
// property omits the install strategy. Reading it therefore costs one small
// pull of the bundle image, so the resolver is deliberately lazy. It is asked
// only when a copy has already failed because no instance matched the
// requested platforms, which keeps a catalog of thousands of bundles free of
// any bundle pulls as long as its images mirror cleanly.
//
// Results are memoised per bundle, and concurrent callers asking about the same
// bundle wait on a single pull.
type OperandResolver struct {
	Log      clog.PluggableLoggerInterface
	Opts     *mirror.CopyOptions
	handler  bundleHandlerInterface
	mu       sync.Mutex
	bundles  map[string]*bundleControllers
	initOnce sync.Once
}

// bundleControllers holds the images one bundle runs itself, fetched at most
// once however many goroutines ask for it.
type bundleControllers struct {
	once   sync.Once
	images map[string]struct{}
	err    error
}

// NewOperandResolver builds a resolver that reads bundles from the registry.
func NewOperandResolver(
	log clog.PluggableLoggerInterface,
	opts *mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) *OperandResolver {
	return &OperandResolver{
		Log:  log,
		Opts: opts,
		handler: BundleHandler{
			Log:      log,
			Mirror:   mirror,
			Manifest: manifest,
		},
		bundles: make(map[string]*bundleControllers),
	}
}

// IsOperand reports whether imageOrigin is an operand of the bundles that list
// it. bundles maps a bundle image reference to its name, exactly as
// CollectorSchema.CopyImageSchemaMap.BundlesByImage records it.
//
// An image that any of those bundles runs itself is not an operand: that
// operator needs it, so failing to mirror it has to be an error.
func (r *OperandResolver) IsOperand(ctx context.Context, imageOrigin string, bundles map[string]string) (bool, error) {
	if len(bundles) == 0 {
		// Not linked to a bundle, so there is nothing to prove it is an
		// operand. Treat it as an image the operator needs.
		return false, nil
	}

	ref := referenceWithoutTransport(imageOrigin)

	for bundleImage := range bundles {
		controllers, err := r.controllersOf(ctx, bundleImage)
		if err != nil {
			return false, err
		}
		if _, runsIt := controllers[ref]; runsIt {
			return false, nil
		}
	}

	return true, nil
}

// controllersOf returns the images the given bundle runs itself, pulling and
// parsing the bundle at most once.
func (r *OperandResolver) controllersOf(ctx context.Context, bundleImage string) (map[string]struct{}, error) {
	r.initOnce.Do(func() {
		if r.bundles == nil {
			r.bundles = make(map[string]*bundleControllers)
		}
	})

	r.mu.Lock()
	entry, found := r.bundles[bundleImage]
	if !found {
		entry = &bundleControllers{}
		r.bundles[bundleImage] = entry
	}
	r.mu.Unlock()

	entry.once.Do(func() {
		dir := filepath.Join(r.Opts.Global.WorkingDir, operatorBundlesDir, bundleDirName(bundleImage))
		r.Log.Debug(collectorPrefix+"reading bundle %s to identify its operands", bundleImage)
		entry.images, entry.err = r.handler.ControllerImages(ctx, bundleImage, dir, *r.Opts)
	})

	return entry.images, entry.err
}

// referenceWithoutTransport strips the transport prefix so an image origin can
// be compared with the plain references a ClusterServiceVersion holds.
func referenceWithoutTransport(ref string) string {
	if _, after, found := strings.Cut(ref, "://"); found {
		return after
	}
	return ref
}

// bundleDirName derives a stable, filesystem-safe directory name for a bundle.
func bundleDirName(bundleImage string) string {
	if spec, err := image.ParseRef(bundleImage); err == nil && spec.Digest != "" {
		return spec.Digest
	}
	return strings.NewReplacer("/", "-", ":", "-", "@", "-").Replace(referenceWithoutTransport(bundleImage))
}
