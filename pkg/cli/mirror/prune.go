package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/admin/prune/imageprune"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

// pruneRegistry plans and executes registry pruning based on current and previous Associations.
func (o *MirrorOptions) pruneRegistry(ctx context.Context, prev, curr image.AssociationSet) error {
	deleter, toRemove, err := o.planImagePruning(ctx, curr, prev)
	if err != nil {
		return err
	}
	// We can use MaxPerRegistry for maxWorkers because
	// we only prune from one registry
	return o.pruneImages(deleter, toRemove, o.MaxPerRegistry)
}

// planImagePruning creates a ManifestDeleter and map of manifests scheduled for deletion.
func (o *MirrorOptions) planImagePruning(ctx context.Context, curr, prev image.AssociationSet) (imageprune.ManifestDeleter, map[string][]string, error) {
	var insecure bool
	if o.DestPlainHTTP || o.DestSkipTLS {
		insecure = true
	}
	deleter := NewManifestDeleter(ctx, o.Out, o.ErrOut, o.ToMirror, insecure)
	manifestsByRepo := map[string][]string{}

	// We compare repo locations to allow the translation between
	// mirror-to-mirror and disk-to-mirror association paths.
	getRepoLoc := func(assocPath string) (string, error) {
		ref, err := reference.Parse(assocPath)
		if err != nil {
			return "", fmt.Errorf("invalid association set")
		}

		var repoLoc string
		// If the imageAssoc path is the location
		// in the target registry (i.e. mirror to mirror), unset the
		// registry information and use the repo location as is.
		if ref.Registry != "" {
			ref.Registry = ""
			repoLoc = ref.AsRepository().String()
		} else {
			repoLoc = path.Join(o.UserNamespace, ref.AsRepository().String())
		}

		return repoLoc, nil
	}

	keyforUniqueName := func(assoc v1alpha2.Association) (string, error) {
		// Combine the source image or child manifest digest with the
		// target location.
		repoLoc, err := getRepoLoc(assoc.Path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%v-%v", assoc.ID, repoLoc), nil
	}

	// Gather all manifests that exists
	// current set.
	currSet := map[string]v1alpha2.Association{}
	for _, assocs := range curr {
		for _, assoc := range assocs {
			unique, err := keyforUniqueName(assoc)
			if err != nil {
				return deleter, manifestsByRepo, err
			}
			currSet[unique] = assoc
		}

	}

	outputSet := map[string]v1alpha2.Association{}
	for _, assocs := range prev {
		for _, assoc := range assocs {
			unique, err := keyforUniqueName(assoc)
			if err != nil {
				return deleter, manifestsByRepo, err
			}
			if _, exists := currSet[unique]; exists {
				// Do not add to the output set if the manifest
				// exists for the image in the current set
				continue
			}
			outputSet[unique] = assoc
		}
	}

	for _, assoc := range outputSet {

		// We are only processing keys where we have
		// access to the manifest digest. Associated
		// tags will be deleted with the manifest.
		if assoc.ID == "" {
			continue
		}

		repoLoc, err := getRepoLoc(assoc.Path)
		if err != nil {
			return deleter, manifestsByRepo, err
		}

		manifests := manifestsByRepo[repoLoc]
		manifests = append(manifests, assoc.ID)
		sortManifests(manifests)
		manifestsByRepo[repoLoc] = manifests
	}
	return deleter, manifestsByRepo, nil
}

// pruneImages performs the image deletion based on the provided map of repos and manifests.
func (o *MirrorOptions) pruneImages(deleter imageprune.ManifestDeleter, manifestsByRepo map[string][]string, maxWorkers int) error {
	if len(manifestsByRepo) == 0 {
		klog.V(2).Info("No images specified for pruning")
		return nil
	}

	klog.Infof("Pruning %d image(s) from registry", len(manifestsByRepo))

	var keys []string
	for k := range manifestsByRepo {
		keys = append(keys, k)
	}

	var wg sync.WaitGroup
	var mutex sync.Mutex
	workQueue := make(chan string)
	errorsCh := make(chan error)

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range workQueue {
				mutex.Lock()
				manifests := manifestsByRepo[k]
				mutex.Unlock()

				for _, manifest := range manifests {
					err := deleter.DeleteManifest(k, manifest)
					if err != nil {
						err = fmt.Errorf("repo %q manifest %s: %w", k, manifest, err)
						errorsCh <- err
					}
				}
			}
		}()
	}

	go func() {
		for _, k := range keys {
			workQueue <- k
		}
		close(workQueue)
		wg.Wait()
		close(errorsCh)
	}()

	var errs []error

	skipErr := func(err error) bool {
		var terr *transport.Error
		return errors.As(err, &terr)
	}

	logMessage := func(err error) string {
		var terr *transport.Error
		if errors.As(err, &terr) {
			switch terr.StatusCode {
			case 405:
				return fmt.Sprintf("image deletion is not permitted on registry %s: %v", o.ToMirror, err.Error())
			case 401:
				return fmt.Sprintf("unauthorized image deletion on registry %s: %v", o.ToMirror, err.Error())
			default:
				return err.Error()
			}
		}
		return err.Error()
	}

	for err := range errorsCh {
		errs = append(errs, o.checkErr(err, skipErr, logMessage))
	}

	return utilerrors.NewAggregate(errs)
}

type pruneImagePlan struct {
	Registry     string       `json:"registry,omitempty"`
	Repositories []repository `json:"repositories,omitempty"`
}

type repository struct {
	Name      string   `json:"name,omitempty"`
	Manifests []string `json:"manifests,omitempty"`
}

// outputPruneImagePlan will write a plan for pruning images to disk.
func (o *MirrorOptions) outputPruneImagePlan(ctx context.Context, prev, curr image.AssociationSet) error {
	_, toRemove, err := o.planImagePruning(ctx, curr, prev)
	if err != nil {
		return err
	}
	if len(toRemove) == 0 {
		klog.V(2).Info("No images planned for pruning")
		return nil
	}
	planFilePath := filepath.Join(o.Dir, "pruning-plan.json")
	cleanPlanFilePath := filepath.Clean(planFilePath)
	klog.Infof("Writing image pruning plan to %s", planFilePath)
	planFile, err := os.Create(cleanPlanFilePath)
	if err != nil {
		return err
	}
	defer planFile.Close()

	plan := aggregateImageInformation(o.ToMirror, toRemove)

	if err := writePruneImagePlan(planFile, plan); err != nil {
		return err
	}

	return planFile.Sync()
}

// writePruneImagePlan will write the prune image plan in JSON format.
func writePruneImagePlan(w io.Writer, plan pruneImagePlan) error {
	data, err := json.MarshalIndent(&plan, "", " ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// aggregateImageInformation will create a prune image plan from registry
// and manifest information.
func aggregateImageInformation(registry string, manifestsByRepo map[string][]string) pruneImagePlan {
	plan := pruneImagePlan{}
	plan.Registry = registry

	for repo, manifests := range manifestsByRepo {
		r := repository{
			Name:      repo,
			Manifests: manifests,
		}
		sortManifests(r.Manifests)
		plan.Repositories = append(plan.Repositories, r)
	}
	sortRepos(plan.Repositories)
	return plan
}

func sortRepos(repos []repository) {
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
}

func sortManifests(manifests []string) {
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i] < manifests[j]
	})
}

// manifestDeleter prints information about each repo manifest being
// deleted. Implement the ManifestDeleter interface for `oc adm prune images``.
// TODO(jpower432): Factor out go-containerregistry in favor of the concrete types
// defined in this imageprune package.
type manifestDeleter struct {
	w        io.Writer
	errOut   io.Writer
	nopts    []name.Option
	ropts    []remote.Option
	registry string
}

var _ imageprune.ManifestDeleter = &manifestDeleter{}

// NewManifestDeleter create a new implementation of the ManifestDeleter interface
func NewManifestDeleter(ctx context.Context, w, errOut io.Writer, registry string, insecure bool) imageprune.ManifestDeleter {
	getNameOpts(insecure)
	return &manifestDeleter{
		w:        w,
		errOut:   errOut,
		nopts:    getNameOpts(insecure),
		ropts:    getRemoteOpts(ctx, insecure),
		registry: registry,
	}
}

// DeleteManifest deletes manifest from a repository.
func (p *manifestDeleter) DeleteManifest(repo, manifest string) error {
	fmt.Fprintf(p.w, "Deleting manifest %s from repo %s\n", manifest, repo)
	ref := path.Join(p.registry, repo)
	ref = fmt.Sprintf("%s@%s", ref, manifest)

	nameRef, err := name.ParseReference(ref, p.nopts...)
	if err != nil {
		return fmt.Errorf("error parsing image reference %s: %v", ref, err)
	}

	return remote.Delete(nameRef, p.ropts...)
}
