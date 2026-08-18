package helm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/mitchellh/copystructure"
	"github.com/otiai10/copy"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	helmcli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/releaseutil"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

var (
	lsc     *LocalStorageCollector
	wClient webClient
)

type HelmOptions struct {
	settings *helmcli.EnvSettings
	insecure bool
}

type Downloaders struct {
	indexDownloader indexDownloader
	chartDownloader chartDownloader
}

type ChartDownloaderWrapper struct {
	inner *downloader.ChartDownloader
}

type LocalStorageCollector struct {
	Log                clog.PluggableLoggerInterface
	Config             v2alpha1.ImageSetConfiguration
	Opts               mirror.CopyOptions
	destReg            string
	Helm               *HelmOptions
	Downloaders        Downloaders
	cleanup            func()
	generateV1DestTags bool
}

func NewHelmOptions(tlsVerify bool) *HelmOptions {
	return &HelmOptions{
		settings: helmcli.New(),
		insecure: !tlsVerify,
	}
}

func WithV1Tags(o CollectorInterface) CollectorInterface {
	switch impl := o.(type) {
	case *LocalStorageCollector:
		impl.generateV1DestTags = true
	}
	return o
}

func (o *LocalStorageCollector) HelmImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {
	var allImages []v2alpha1.CopyImageSchema
	var platformFilters map[string][]v2alpha1.InstancePlatformFilter
	var errs []error

	switch {
	case lsc.Opts.IsMirrorToDisk() || lsc.Opts.IsMirrorToMirror():
		defer lsc.cleanup()
		allImages, platformFilters, errs = lsc.collectHelmImagesM2D()
	case lsc.Opts.IsDiskToMirror():
		allImages, platformFilters, errs = lsc.collectHelmImagesD2M(o.generateV1DestTags)
	}

	cs := v2alpha1.CollectorSchema{AllImages: allImages}
	if len(platformFilters) > 0 {
		cs.PlatformFilters = platformFilters
	}
	return cs, errors.Join(errs...)
}

func (lsc *LocalStorageCollector) collectHelmImagesM2D() ([]v2alpha1.CopyImageSchema, map[string][]v2alpha1.InstancePlatformFilter, []error) { //nolint:cyclop
	var allHelmImages []v2alpha1.RelatedImage
	var errs []error
	platformFilters := make(map[string][]v2alpha1.InstancePlatformFilter)

	imgs, localPlatforms, errors := getHelmImagesFromLocalChart()
	errs = append(errs, errors...)
	if len(imgs) > 0 {
		allHelmImages = append(allHelmImages, imgs...)
		maps.Copy(platformFilters, localPlatforms)
	}

	for _, repo := range lsc.Config.Mirror.Helm.Repositories {
		charts := repo.Charts
		if err := repoAdd(repo); err != nil {
			errs = append(errs, err)
			continue
		}
		if charts == nil {
			var indexFile helmrepo.IndexFile
			var err error
			if indexFile, err = createIndexFile(repo.URL); err != nil {
				errs = append(errs, err)
				continue
			}
			if charts, err = getChartsFromIndex("", indexFile); err != nil && charts == nil {
				errs = append(errs, err)
				continue
			}
		}
		for _, chart := range charts {
			lsc.Log.Debug("Pulling chart %s", chart.Name)
			ref := fmt.Sprintf("%s/%s", repo.Name, chart.Name)
			dest := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmChartDir)
			path, _, err := lsc.Downloaders.chartDownloader.DownloadTo(ref, chart.Version, dest)
			if err != nil {
				errs = append(errs, err)
				lsc.Log.Error("error pulling chart %s:%s", ref, err.Error())
				continue
			}
			if err := v2alpha1.ValidatePlatforms(chart.Platforms); err != nil {
				errs = append(errs, fmt.Errorf("invalid platform for chart %q: %w", chart.Name, err))
				continue
			}
			chart, err = prepareChartValuesFiles(chart, true)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			chartImgs, err := getImages(path, chart)
			if err != nil {
				errs = append(errs, err)
			}
			allHelmImages = append(allHelmImages, chartImgs...)
			addChartPlatformFilters(platformFilters, chartImgs, chart.Platforms)
		}
	}

	allImages, err := prepareM2DCopyBatch(allHelmImages)
	if err != nil {
		lsc.Log.Error(errMsg, err.Error())
		errs = append(errs, err)
	}
	return allImages, platformFilters, errs
}

func (lsc *LocalStorageCollector) collectHelmImagesD2M(generateV1Tags bool) ([]v2alpha1.CopyImageSchema, map[string][]v2alpha1.InstancePlatformFilter, []error) {
	var allHelmImages []v2alpha1.RelatedImage
	var errs []error
	platformFilters := make(map[string][]v2alpha1.InstancePlatformFilter)

	imgs, localPlatforms, errors := getHelmImagesFromLocalChart()
	errs = append(errs, errors...)
	allHelmImages = append(allHelmImages, imgs...)
	maps.Copy(platformFilters, localPlatforms)

	for _, repo := range lsc.Config.Mirror.Helm.Repositories {
		charts, err := resolveChartsForRepo(repo)
		if err != nil {
			errs = append(errs, err)
		}
		if charts == nil {
			continue
		}
		for _, chart := range charts {
			chartDir := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmChartDir)
			path, err := resolveChartPath(chartDir, chart.Name, chart.Version)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := v2alpha1.ValidatePlatforms(chart.Platforms); err != nil {
				errs = append(errs, fmt.Errorf("invalid platform for chart %q: %w", chart.Name, err))
				continue
			}
			chart, err = prepareChartValuesFiles(chart, false)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			chartImgs, err := getImages(path, chart)
			if err != nil {
				errs = append(errs, err)
			}
			allHelmImages = append(allHelmImages, chartImgs...)
			addChartPlatformFilters(platformFilters, chartImgs, chart.Platforms)
		}
	}

	allImages, err := prepareD2MCopyBatch(allHelmImages, generateV1Tags)
	if err != nil {
		lsc.Log.Error(errMsg, err.Error())
		errs = append(errs, err)
	}
	return allImages, platformFilters, errs
}

func addChartPlatformFilters(filters map[string][]v2alpha1.InstancePlatformFilter, imgs []v2alpha1.RelatedImage, platforms []v2alpha1.InstancePlatformFilter) {
	if len(platforms) == 0 {
		return
	}
	for _, img := range imgs {
		ref, err := image.ParseRef(img.Image)
		if err != nil {
			continue
		}
		origin := ref.ReferenceWithTransport
		filters[origin] = append(filters[origin], platforms...)
	}
}

func resolveChartsForRepo(repo v2alpha1.Repository) ([]v2alpha1.Chart, error) {
	if repo.Charts != nil {
		return repo.Charts, nil
	}
	return getChartsFromIndex(repo.URL, helmrepo.IndexFile{})
}

func createTempFile(dir string) (func(), string, error) {
	file, err := os.CreateTemp(dir, "repo.*")
	return func() {
		if err := os.Remove(file.Name()); err != nil {
			lsc.Log.Error("%s", err.Error())
		}
	}, file.Name(), err
}

func (cdw *ChartDownloaderWrapper) DownloadTo(ref, version, dest string) (string, any, error) {
	return cdw.inner.DownloadTo(ref, version, dest)
}

func GetDefaultChartDownloader() chartDownloader {
	lsc.Log.Debug("GetDefaultChartDownloader - lsc.Helm.insecure %t", lsc.Helm.insecure)
	return &ChartDownloaderWrapper{
		inner: &downloader.ChartDownloader{
			Out:     lsc.Opts.Stdout,
			Verify:  downloader.VerifyNever,
			Getters: getter.All(lsc.Helm.settings),
			Options: []getter.Option{
				getter.WithInsecureSkipVerifyTLS(lsc.Helm.insecure),
			},
			RepositoryConfig: lsc.Helm.settings.RepositoryConfig,
			RepositoryCache:  lsc.Helm.settings.RepositoryCache,
		},
	}
}

func getHelmImagesFromLocalChart() ([]v2alpha1.RelatedImage, map[string][]v2alpha1.InstancePlatformFilter, []error) {
	var allHelmImages []v2alpha1.RelatedImage
	var errs []error
	platformFilters := make(map[string][]v2alpha1.InstancePlatformFilter)

	for _, chart := range lsc.Config.Mirror.Helm.Local {
		if err := v2alpha1.ValidatePlatforms(chart.Platforms); err != nil {
			errs = append(errs, fmt.Errorf("invalid platform for chart %q: %w", chart.Name, err))
			continue
		}

		// Persist values files during mirror-to-disk / mirror-to-mirror so
		// disk-to-mirror can re-render without the original host paths.
		chart, err := prepareChartValuesFiles(chart, !lsc.Opts.IsDiskToMirror())
		if err != nil {
			errs = append(errs, err)
			continue
		}

		imgs, err := getImages(chart.Path, chart)
		if err != nil {
			errs = append(errs, err)
		}

		if len(imgs) == 0 {
			continue
		}
		allHelmImages = append(allHelmImages, imgs...)
		addChartPlatformFilters(platformFilters, imgs, chart.Platforms)
	}

	return allHelmImages, platformFilters, errs
}

func repoAdd(chartRepo v2alpha1.Repository) error {
	entry := helmrepo.Entry{
		Name: chartRepo.Name,
		URL:  chartRepo.URL,
	}

	var err error
	var helmFile helmrepo.File
	helmFile, err = parser.ParseYamlFile[helmrepo.File](lsc.Helm.settings.RepositoryConfig)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("parse helm repo config: %w", err)
	}

	// Check for existing repo name
	if helmFile.Has(chartRepo.Name) {
		lsc.Log.Info("repository name (%s) already exists", chartRepo.Name)
		return nil
	}

	var indexDownloader indexDownloader
	if lsc.Downloaders.indexDownloader == nil {
		indexDownloader, err = helmrepo.NewChartRepository(&entry, getter.All(lsc.Helm.settings))
		if err != nil {
			msg := strings.ReplaceAll(err.Error(), "for:", "")
			return fmt.Errorf("setting index downloader for %s: %s", strings.TrimSpace(chartRepo.Name), msg)
		}
	} else {
		indexDownloader = lsc.Downloaders.indexDownloader
	}

	if _, err := indexDownloader.DownloadIndexFile(); err != nil {
		return fmt.Errorf("invalid chart repository %q: %w", chartRepo.URL, err)
	}

	// Update temp file with chart entry
	helmFile.Update(&entry)

	if err := helmFile.WriteFile(lsc.Helm.settings.RepositoryConfig, 0644); err != nil {
		return fmt.Errorf("error writing helm repo file: %s %w", strings.TrimSpace(chartRepo.Name), err)
	}

	return nil
}

func createIndexFile(indexURL string) (helmrepo.IndexFile, error) {
	if !strings.HasSuffix(indexURL, "/index.yaml") {
		indexURL += "index.yaml"
	}
	resp, err := wClient.Get(indexURL)
	if err != nil {
		return helmrepo.IndexFile{}, fmt.Errorf("request helm index: %s %w", indexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return helmrepo.IndexFile{}, fmt.Errorf("response for %v returned %v with status code %v", indexURL, resp, resp.StatusCode)
	}

	indexFile, err := parser.ParseYamlReader[helmrepo.IndexFile](resp.Body)
	if err != nil {
		return helmrepo.IndexFile{}, fmt.Errorf("failed to parse %q into index file: %w", indexURL, err)
	}

	namespace := getNamespaceFromURL(indexURL)

	indexDir := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmIndexesDir, namespace)

	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return indexFile, err
	}

	indexFilePath := filepath.Join(indexDir, "index.yaml")

	if err := indexFile.WriteFile(indexFilePath, 0644); err != nil {
		return indexFile, fmt.Errorf("error writing helm index file: %s", err.Error())
	}

	return indexFile, nil
}

func getNamespaceFromURL(url string) string {
	pathSplit := strings.Split(url, "/")
	return strings.Join(pathSplit[2:len(pathSplit)-1], "/")
}

func getChartsFromIndex(indexURL string, indexFile helmrepo.IndexFile) ([]v2alpha1.Chart, error) {
	var charts []v2alpha1.Chart

	if lsc.Opts.IsDiskToMirror() {
		namespace := getNamespaceFromURL(indexURL)

		indexFilePath := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmIndexesDir, namespace, helmIndexFile)

		var err error
		indexFile, err = parser.ParseYamlFile[helmrepo.IndexFile](indexFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", indexFilePath, err)
		}
	}

	for key, chartVersions := range indexFile.Entries {
		for _, chartVersion := range chartVersions {
			if chartVersion.Type != "library" {
				charts = append(charts, v2alpha1.Chart{Name: key, Version: chartVersion.Version})
			}
		}
	}
	return charts, nil
}

// resolveChartPath returns the filesystem path to a Helm chart tarball stored
// under dir.  It normalises the version with semver (which strips any leading
// "v") and then probes two candidate filenames — "{name}-{canonical}.tgz" and
// "{name}-v{canonical}.tgz" — because Helm repositories differ on whether they
// embed the "v" prefix in tarball URLs.  A descriptive error naming both
// attempted paths is returned when neither file exists.
func resolveChartPath(dir, name, version string) (string, error) {
	ver, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("invalid chart version %q: %w", version, err)
	}
	canonical := ver.String() // always without "v" prefix, regardless of how version was specified

	baseDir := filepath.Clean(dir)
	candidateVersions := []string{canonical, "v" + canonical}
	var candidatePaths []string

	for _, candidateVer := range candidateVersions {
		path, err := buildChartCandidatePath(baseDir, name, candidateVer)
		if err != nil {
			return "", err
		}
		candidatePaths = append(candidatePaths, path)

		found, err := chartPathExists(path)
		if err != nil {
			return "", err
		}
		if found {
			if candidateVer != candidateVersions[0] {
				lsc.Log.Debug("chart %s: %s not found, using %s (v-prefix variant)", name, filepath.Base(candidatePaths[0]), filepath.Base(path))
			}
			return path, nil
		}
	}

	return "", fmt.Errorf("chart file not found for %s version %s: tried %s and %s",
		name, version, filepath.Base(candidatePaths[0]), filepath.Base(candidatePaths[1]))
}

// chartPathExists reports whether the given path exists on disk.
// It returns false (without error) for missing files and propagates
// real I/O or permission errors so they are not silently swallowed.
func chartPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

// buildChartCandidatePath constructs the expected tarball path for a chart
// version and validates it stays within baseDir to prevent path traversal.
func buildChartCandidatePath(baseDir, name, ver string) (string, error) {
	p := filepath.Join(baseDir, fmt.Sprintf("%s-%s.tgz", name, ver))
	rel, err := filepath.Rel(baseDir, p)
	if err != nil {
		return "", fmt.Errorf("resolve chart path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid chart reference %q:%q escapes chart directory", name, ver)
	}
	return p, nil
}

func getImages(path string, chart v2alpha1.Chart) (images []v2alpha1.RelatedImage, err error) {
	lsc.Log.Debug("Reading from path %s", path)

	p := getImagesPath(chart.ImagePaths...)

	var helmChart *helmchart.Chart
	if helmChart, err = loader.Load(path); err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}

	valueOpts, err := mergeChartValues(chart)
	if err != nil {
		return nil, fmt.Errorf("failed to load values for chart %s: %w", chart.Name, err)
	}

	var templates string
	if templates, err = getHelmTemplates(helmChart, valueOpts); err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", helmChart.Name(), err)
	}

	// Process each YAML document seperately
	for _, templateData := range bytes.Split([]byte(templates), []byte("\n---\n")) {
		imgs, err := findImages(templateData, p...)
		if err != nil {
			return nil, err
		}

		images = append(images, imgs...)
	}

	return images, nil
}

// mergeChartValues loads ValuesFiles and merges them with inline Values.
// Later values files override earlier ones; inline Values override files.
// This mirrors `helm template -f file1 -f file2 --set ...` precedence.
func mergeChartValues(chart v2alpha1.Chart) (map[string]any, error) {
	vals := make(map[string]any)

	for _, valuesFile := range chart.ValuesFiles {
		fileVals, err := chartutil.ReadValuesFile(valuesFile)
		if err != nil {
			return nil, fmt.Errorf("read values file %q: %w", valuesFile, err)
		}
		// CoalesceTables keeps destination keys when both maps define the same path,
		// so the newer file must be the destination for later overrides to win.
		vals = chartutil.CoalesceTables(fileVals, vals)
	}

	if len(chart.Values) > 0 {
		// Deep-copy so CoalesceTables does not mutate the shared ImageSetConfiguration map.
		copied, err := copystructure.Copy(chart.Values)
		if err != nil {
			return nil, fmt.Errorf("copy inline values: %w", err)
		}
		inlineVals, ok := copied.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("copy inline values: unexpected type %T", copied)
		}
		vals = chartutil.CoalesceTables(inlineVals, vals)
	}

	return vals, nil
}

// prepareChartValuesFiles rewrites chart.ValuesFiles to paths under the working
// directory. When persist is true (mirror-to-disk / mirror-to-mirror), source
// files are copied into the working dir so they are included in the archive.
// When persist is false (disk-to-mirror), the previously persisted copies are used.
func prepareChartValuesFiles(chart v2alpha1.Chart, persist bool) (v2alpha1.Chart, error) {
	if len(chart.ValuesFiles) == 0 {
		return chart, nil
	}

	destDir := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmValuesDir, chartValuesDirName(chart))
	updated := make([]string, 0, len(chart.ValuesFiles))

	for i, src := range chart.ValuesFiles {
		dest := filepath.Join(destDir, fmt.Sprintf("%d-%s", i, filepath.Base(src)))
		if persist {
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return chart, fmt.Errorf("create values dir for chart %q: %w", chart.Name, err)
			}
			if err := copy.Copy(src, dest); err != nil {
				return chart, fmt.Errorf("persist values file %q for chart %q: %w", src, chart.Name, err)
			}
		} else if _, err := os.Stat(dest); err != nil {
			return chart, fmt.Errorf("persisted values file for chart %q not found at %q (required for disk-to-mirror): %w", chart.Name, dest, err)
		}
		updated = append(updated, dest)
	}

	chart.ValuesFiles = updated
	return chart, nil
}

func chartValuesDirName(chart v2alpha1.Chart) string {
	// Hash name/version/values-file paths so directories stay unique across repos
	// and cannot escape the working directory via crafted chart names.
	key := chart.Name + "\x00" + chart.Version + "\x00" + strings.Join(chart.ValuesFiles, "\x00")
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%s-%s-%s", sanitizePathComponent(chart.Name), sanitizePathComponent(defaultString(chart.Version, "unversioned")), hex.EncodeToString(sum[:8]))
}

func sanitizePathComponent(s string) string {
	s = filepath.Base(filepath.Clean(s))
	switch s {
	case ".", "..", string(filepath.Separator), "":
		return "chart"
	default:
		return strings.ReplaceAll(s, string(filepath.Separator), "_")
	}
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// getImagesPath returns known jsonpaths and user defined jsonpaths where images are found
// it follows the pattern of jsonpath library which is different from text/template
func getImagesPath(paths ...string) []string {
	pathlist := []string{
		"{.spec.template.spec.initContainers[*].image}",
		"{.spec.template.spec.containers[*].image}",
		"{.spec.initContainers[*].image}",
		"{.spec.containers[*].image}",
	}
	return append(pathlist, paths...)
}

// getHelmTemplates returns all chart templates
func getHelmTemplates(ch *helmchart.Chart, valueOpts map[string]any) (string, error) {
	out := new(bytes.Buffer)
	caps := chartutil.DefaultCapabilities

	// Match v1 rendering: use non-empty placeholders so charts that concatenate
	// .Release.Name into metadata fields still produce valid YAML. Charts that
	// embed .Release.Name into image references are uncommon; prefer overriding
	// those image values via values/valuesFiles when needed.
	relOps := chartutil.ReleaseOptions{
		Name:      "NAME",
		Namespace: "RELEASE-NAMESPACE",
	}

	if err := chartutil.ProcessDependencies(ch, valueOpts); err != nil {
		return "", fmt.Errorf("error processing dependencies: %w", err)
	}

	valuesToRender, err := chartutil.ToRenderValues(ch, valueOpts, relOps, caps)
	if err != nil {
		return "", fmt.Errorf("error rendering values: %w", err)
	}

	files, err := engine.Render(ch, valuesToRender)
	if err != nil {
		return "", fmt.Errorf("error rendering chart %s: %w", ch.Name(), err)
	}

	dropNotesFiles(files)
	writeCRDs(out, ch)
	return writeRenderedManifests(out, files, caps)
}

func dropNotesFiles(files map[string]string) {
	for k := range files {
		if strings.HasSuffix(k, ".txt") {
			delete(files, k)
		}
	}
}

func writeCRDs(out *bytes.Buffer, ch *helmchart.Chart) {
	for _, crd := range ch.CRDObjects() {
		fmt.Fprintf(out, "---\n# Source: %s\n%s\n", crd.Name, string(crd.File.Data[:]))
	}
}

func writeRenderedManifests(out *bytes.Buffer, files map[string]string, caps *chartutil.Capabilities) (string, error) {
	_, manifests, err := releaseutil.SortManifests(files, caps.APIVersions, releaseutil.InstallOrder)
	if err != nil {
		// Return the files as a blob to help debug parser errors.
		for name, content := range files {
			if strings.TrimSpace(content) == "" {
				continue
			}
			fmt.Fprintf(out, "---\n# Source: %s\n%s\n", name, content)
		}
		return out.String(), err
	}
	for _, m := range manifests {
		fmt.Fprintf(out, "---\n# Source: %s\n%s\n", m.Name, m.Content)
	}
	return out.String(), nil
}

// findImages will return images from parsed object
func findImages(templateData []byte, paths ...string) (images []v2alpha1.RelatedImage, err error) {
	var data any
	if err := yaml.Unmarshal(templateData, &data); err != nil {
		return nil, err
	}

	j := jsonpath.New("")
	j.AllowMissingKeys(true)

	for _, path := range paths {
		results, err := parseJSONPath(data, j, path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		for _, result := range results {
			ref, err := image.ParseRef(result)
			if err != nil {
				lsc.Log.Debug("invalid helm image: %s", result)
				continue
			}

			lsc.Log.Debug("Found image %s", result)
			img := v2alpha1.RelatedImage{
				Image: ref.ReferenceWithTransport,
				Type:  v2alpha1.TypeHelmImage,
			}

			images = append(images, img)
		}
	}

	return images, nil
}

// parseJSONPath will parse data and filter for a provided jsonpath template
func parseJSONPath(input any, parser *jsonpath.JSONPath, template string) ([]string, error) {
	buf := new(bytes.Buffer)
	if err := parser.Parse(template); err != nil {
		return nil, err
	}
	if err := parser.Execute(buf, input); err != nil {
		return nil, err
	}

	f := func(s rune) bool { return s == ' ' }
	r := strings.FieldsFunc(buf.String(), f)
	return r, nil
}

func prepareM2DCopyBatch(images []v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			lsc.Log.Error("%s", err.Error())
			return nil, err
		}
		src = imgSpec.ReferenceWithTransport

		if imgSpec.IsImageByDigestOnly() {
			tag := fmt.Sprintf("%s-%s", imgSpec.Algorithm, imgSpec.Digest)
			if len(tag) > 128 {
				tag = tag[:127]
			}
			dest = fmt.Sprintf("%s%s/%s:%s", consts.DockerProtocol, destinationRegistry(), imgSpec.PathComponent, tag)
		} else if imgSpec.IsImageByTagAndDigest() {
			src = fmt.Sprintf("%s%s/%s@%s:%s", imgSpec.Transport, imgSpec.Domain, imgSpec.PathComponent, imgSpec.Algorithm, imgSpec.Digest)
			dest = fmt.Sprintf("%s%s/%s:%s", consts.DockerProtocol, destinationRegistry(), imgSpec.PathComponent, imgSpec.Tag)
		} else {
			dest = fmt.Sprintf("%s%s/%s:%s", consts.DockerProtocol, destinationRegistry(), imgSpec.PathComponent, imgSpec.Tag)
		}

		lsc.Log.Debug("source %s", src)
		lsc.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{
			Origin:      imgSpec.ReferenceWithTransport,
			Source:      src,
			Destination: dest,
			Type:        img.Type,
		})
	}
	return result, nil
}

func prepareD2MCopyBatch(images []v2alpha1.RelatedImage, generateV1TagsFromDigests bool) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			lsc.Log.Error("%s", err.Error())
			return nil, err
		}
		if imgSpec.IsImageByDigestOnly() {
			tag := fmt.Sprintf("%s-%s", imgSpec.Algorithm, imgSpec.Digest)
			if len(tag) > 128 {
				tag = tag[:127]
			}
			src = fmt.Sprintf("%s%s/%s:%s", consts.DockerProtocol, lsc.Opts.LocalStorageFQDN, imgSpec.PathComponent, tag)
			if generateV1TagsFromDigests {
				dest = fmt.Sprintf("%s/%s:%s", lsc.Opts.Destination, imgSpec.PathComponent, "latest")
			} else {
				dest = fmt.Sprintf("%s/%s:%s", lsc.Opts.Destination, imgSpec.PathComponent, tag)
			}
		} else if imgSpec.IsImageByTagAndDigest() {
			src = fmt.Sprintf("%s%s/%s@%s:%s", imgSpec.Transport, lsc.Opts.LocalStorageFQDN, imgSpec.PathComponent, imgSpec.Algorithm, imgSpec.Digest)
			dest = fmt.Sprintf("%s/%s:%s", lsc.Opts.Destination, imgSpec.PathComponent, imgSpec.Tag)
		} else {
			src = fmt.Sprintf("%s%s/%s:%s", consts.DockerProtocol, lsc.Opts.LocalStorageFQDN, imgSpec.PathComponent, imgSpec.Tag)
			dest = fmt.Sprintf("%s/%s:%s", lsc.Opts.Destination, imgSpec.PathComponent, imgSpec.Tag)
		}
		if src == "" || dest == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
		}

		lsc.Log.Debug("source %s", src)
		lsc.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{
			Origin:      imgSpec.ReferenceWithTransport,
			Source:      src,
			Destination: dest,
			Type:        img.Type,
		})

	}
	return result, nil
}

func destinationRegistry() string {
	if lsc.destReg == "" {
		if lsc.Opts.IsDiskToMirror() || lsc.Opts.IsMirrorToMirror() {
			lsc.destReg = strings.TrimPrefix(lsc.Opts.Destination, consts.DockerProtocol)
		} else {
			lsc.destReg = lsc.Opts.LocalStorageFQDN
		}
	}
	return lsc.destReg
}
