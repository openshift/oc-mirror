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
type OperandResolver struct {
	Log      clog.PluggableLoggerInterface
	Opts     *mirror.CopyOptions
	handler  bundleHandlerInterface
	mu       sync.Mutex
	bundles  map[string]*bundleControllers
	initOnce sync.Once
}

// bundleControllers holds the images one bundle runs itself, fetched at most
// once however many goroutines ask for it. specs holds those that parsed; the
// raw strings remain for references ParseRef cannot handle.
type bundleControllers struct {
	once   sync.Once
	images map[string]struct{}
	specs  []image.ImageSpec
	err    error
}

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
		// Nothing proves it is an operand, so treat it as one the operator needs.
		return false, nil
	}

	ref := referenceWithoutTransport(imageOrigin)
	spec, specErr := image.ParseRef(ref)

	for bundleImage := range bundles {
		controllers, err := r.controllersOf(ctx, bundleImage)
		if err != nil {
			return false, err
		}
		if _, runsIt := controllers.images[ref]; runsIt {
			return false, nil
		}
		if specErr != nil {
			continue
		}
		for _, controller := range controllers.specs {
			if sameImage(spec, controller) {
				return false, nil
			}
		}
	}

	return true, nil
}

// sameImage reports whether two parsed references denote the same image. Name
// carries the registry, so the same path on different hosts does not match.
// A tag on one side and a digest on the other cannot be compared, so they are
// treated as a match: the reverse would silently skip an image the operator runs.
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

func (r *OperandResolver) controllersOf(ctx context.Context, bundleImage string) (*bundleControllers, error) {
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
		for controller := range entry.images {
			if spec, err := image.ParseRef(controller); err == nil {
				entry.specs = append(entry.specs, spec)
			}
		}
	})

	if entry.err != nil {
		// A cached failure would leave every other image in this bundle
		// unclassified, so let the next caller retry. Drop it only while it is
		// still the current entry: another goroutine may already have replaced
		// it with a retry of its own.
		r.mu.Lock()
		if r.bundles[bundleImage] == entry {
			delete(r.bundles, bundleImage)
		}
		r.mu.Unlock()
	}

	return entry, entry.err
}

func referenceWithoutTransport(ref string) string {
	if _, after, found := strings.Cut(ref, "://"); found {
		return after
	}
	return ref
}

func bundleDirName(bundleImage string) string {
	if spec, err := image.ParseRef(bundleImage); err == nil && spec.Digest != "" {
		return spec.Digest
	}
	return strings.NewReplacer("/", "-", ":", "-", "@", "-").Replace(referenceWithoutTransport(bundleImage))
}
