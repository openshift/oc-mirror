package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
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
)

var lsc *LocalStorageCollector
var wClient webClient

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
	Log         clog.PluggableLoggerInterface
	Config      v2alpha1.ImageSetConfiguration
	Opts        mirror.CopyOptions
	destReg     string
	Helm        *HelmOptions
	Downloaders Downloaders
	cleanup     func()
}

func NewHelmOptions() *HelmOptions {
	return &HelmOptions{
		settings: helmcli.New(),
		insecure: true,
	}
}

func (o *LocalStorageCollector) HelmImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	var (
		allImages     []v2alpha1.CopyImageSchema
		allHelmImages []v2alpha1.RelatedImage
		errs          []error
	)

	switch {
	case lsc.Opts.IsMirrorToDisk() || lsc.Opts.IsMirrorToMirror():
		defer lsc.cleanup()

		var err error
		imgs, errors := getHelmImagesFromLocalChart()
		if len(errors) > 0 {
			errs = append(errs, errors...)
		}
		if len(imgs) > 0 {
			allHelmImages = append(allHelmImages, imgs...)
		}

		for _, repo := range lsc.Config.Mirror.Helm.Repositories {
			charts := repo.Charts

			if err := repoAdd(repo); err != nil {
				errs = append(errs, err)
				continue
			}

			if charts == nil {
				var indexFile helmrepo.IndexFile
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

				imgs, err := getImages(path, chart.ImagePaths...)
				if err != nil {
					errs = append(errs, err)
				}

				allHelmImages = append(allHelmImages, imgs...)

			}
		}

		allImages, err = prepareM2DCopyBatch(allHelmImages)
		if err != nil {
			lsc.Log.Error(errMsg, err.Error())
			errs = append(errs, err)
		}

	case lsc.Opts.IsDiskToMirror():
		imgs, errors := getHelmImagesFromLocalChart()
		if len(errors) > 0 {
			errs = append(errs, errors...)
		}
		if len(imgs) > 0 {
			allHelmImages = append(allHelmImages, imgs...)
		}

		for _, repo := range lsc.Config.Mirror.Helm.Repositories {
			charts := repo.Charts

			if charts == nil {
				var err error
				if charts, err = getChartsFromIndex(repo.URL, helmrepo.IndexFile{}); err != nil {
					errs = append(errs, err)
					if charts == nil {
						continue
					}
				}
			}

			for _, chart := range charts {
				src := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmChartDir)
				path := filepath.Join(src, fmt.Sprintf("%s-%s.tgz", chart.Name, chart.Version))

				imgs, err := getImages(path, chart.ImagePaths...)
				if err != nil {
					errs = append(errs, err)
				}

				allHelmImages = append(allHelmImages, imgs...)

			}
		}

		var err error
		allImages, err = prepareD2MCopyBatch(allHelmImages)
		if err != nil {
			lsc.Log.Error(errMsg, err.Error())
			errs = append(errs, err)
		}
	}

	return allImages, errors.Join(errs...)
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

func getHelmImagesFromLocalChart() ([]v2alpha1.RelatedImage, []error) {
	var allHelmImages []v2alpha1.RelatedImage
	var errs []error

	for _, chart := range lsc.Config.Mirror.Helm.Local {
		imgs, err := getImages(chart.Path, chart.ImagePaths...)
		if err != nil {
			errs = append(errs, err)
		}

		if len(imgs) > 0 {
			allHelmImages = append(allHelmImages, imgs...)
		}
	}

	return allHelmImages, errs

}

func repoAdd(chartRepo v2alpha1.Repository) error {

	entry := helmrepo.Entry{
		Name: chartRepo.Name,
		URL:  chartRepo.URL,
	}

	b, err := os.ReadFile(lsc.Helm.settings.RepositoryConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var helmFile helmrepo.File
	if err := yaml.Unmarshal(b, &helmFile); err != nil {
		return err
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
			return err
		}
	} else {
		indexDownloader = lsc.Downloaders.indexDownloader
	}

	if _, err := indexDownloader.DownloadIndexFile(); err != nil {
		return fmt.Errorf("invalid chart repository %q: %v", chartRepo.URL, err)
	}

	// Update temp file with chart entry
	helmFile.Update(&entry)

	if err := helmFile.WriteFile(lsc.Helm.settings.RepositoryConfig, 0644); err != nil {
		return fmt.Errorf("error writing helm repo file: %v", err)
	}

	return nil
}

func createIndexFile(indexURL string) (helmrepo.IndexFile, error) {
	var indexFile helmrepo.IndexFile
	if !strings.HasSuffix(indexURL, "/index.yaml") {
		indexURL += "index.yaml"
	}
	resp, err := wClient.Get(indexURL)
	if err != nil {
		return indexFile, err
	}
	if resp.StatusCode != 200 {
		return indexFile, fmt.Errorf("response for %v returned %v with status code %v", indexURL, resp, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return indexFile, err
	}
	err = yaml.Unmarshal(body, &indexFile)
	if err != nil {
		return indexFile, err
	}

	namespace := getNamespaceFromURL(indexURL)

	indexDir := filepath.Join(lsc.Opts.Global.WorkingDir, helmDir, helmIndexesDir, namespace)

	err = os.MkdirAll(indexDir, 0755)

	if err != nil {
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

		data, err := os.ReadFile(indexFilePath)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(data, &indexFile)
		if err != nil {
			return nil, err
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

func getImages(path string, imagePaths ...string) (images []v2alpha1.RelatedImage, err error) {

	lsc.Log.Debug("Reading from path %s", path)

	p := getImagesPath(imagePaths...)

	var chart *helmchart.Chart
	if chart, err = loader.Load(path); err != nil {
		return nil, err
	}

	var templates string
	if templates, err = getHelmTemplates(chart); err != nil {
		return nil, err
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
func getHelmTemplates(ch *helmchart.Chart) (string, error) {
	out := new(bytes.Buffer)
	valueOpts := make(map[string]interface{})
	caps := chartutil.DefaultCapabilities

	valuesToRender, err := chartutil.ToRenderValues(ch, valueOpts, chartutil.ReleaseOptions{}, caps)
	if err != nil {
		return "", fmt.Errorf("error rendering values: %v", err)
	}

	files, err := engine.Render(ch, valuesToRender)
	if err != nil {
		return "", fmt.Errorf("error rendering chart %s: %v", ch.Name(), err)
	}

	// Skip the NOTES.txt files
	for k := range files {
		if strings.HasSuffix(k, ".txt") {
			delete(files, k)
		}
	}

	for _, crd := range ch.CRDObjects() {
		fmt.Fprintf(out, "---\n# Source: %s\n%s\n", crd.Name, string(crd.File.Data[:]))
	}

	_, manifests, err := releaseutil.SortManifests(files, caps.APIVersions, releaseutil.InstallOrder)
	if err != nil {
		// We return the files as a big blob of data to help the user debug parser
		// errors.
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

	var data interface{}
	if err := yaml.Unmarshal(templateData, &data); err != nil {
		return nil, err
	}

	j := jsonpath.New("")
	j.AllowMissingKeys(true)

	for _, path := range paths {
		results, err := parseJSONPath(data, j, path)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			lsc.Log.Debug("Found image %s", result)
			img := v2alpha1.RelatedImage{
				Image: result,
				Type:  v2alpha1.TypeHelmImage,
			}

			images = append(images, img)
		}
	}

	return images, nil
}

// parseJSONPath will parse data and filter for a provided jsonpath template
func parseJSONPath(input interface{}, parser *jsonpath.JSONPath, template string) ([]string, error) {
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
			dest = dockerProtocol + strings.Join([]string{destinationRegistry(), imgSpec.PathComponent + ":" + tag}, "/")
		} else {
			dest = dockerProtocol + strings.Join([]string{destinationRegistry(), imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")
		}

		lsc.Log.Debug("source %s", src)
		lsc.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})
	}
	return result, nil
}

func prepareD2MCopyBatch(images []v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
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
			src = dockerProtocol + strings.Join([]string{lsc.Opts.LocalStorageFQDN, imgSpec.PathComponent + ":" + tag}, "/")
			dest = strings.Join([]string{lsc.Opts.Destination, imgSpec.PathComponent + ":" + tag}, "/")
		} else {
			src = dockerProtocol + strings.Join([]string{lsc.Opts.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			dest = strings.Join([]string{lsc.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
		}
		if src == "" || dest == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
		}

		lsc.Log.Debug("source %s", src)
		lsc.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})

	}
	return result, nil
}

func destinationRegistry() string {
	if lsc.destReg == "" {
		if lsc.Opts.IsDiskToMirror() || lsc.Opts.IsMirrorToMirror() {
			lsc.destReg = strings.TrimPrefix(lsc.Opts.Destination, dockerProtocol)
		} else {
			lsc.destReg = lsc.Opts.LocalStorageFQDN
		}
	}
	return lsc.destReg
}
