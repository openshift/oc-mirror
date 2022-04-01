package mirror

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	helmcli "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
)

type HelmOptions struct {
	*MirrorOptions
	settings *helmcli.EnvSettings
	insecure bool
}

func NewHelmOptions(mo *MirrorOptions) *HelmOptions {
	settings := helmcli.New()
	opts := &HelmOptions{
		MirrorOptions: mo,
		settings:      settings,
	}
	if mo.SourcePlainHTTP || mo.SourceSkipTLS {
		opts.insecure = true
	}
	return opts

}

func (h *HelmOptions) PullCharts(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {

	var images []v1alpha2.Image

	// Create a temp file for to hold repo information
	cleanup, file, err := mktempFile(h.Dir)
	if err != nil {
		return nil, err
	}
	h.settings.RepositoryConfig = file
	defer cleanup()

	// Configure downloader
	// TODO: allow configuration of credentials
	// and certs
	c := downloader.ChartDownloader{
		Out:     os.Stdout,
		Keyring: "",
		Verify:  downloader.VerifyNever,
		Getters: getter.All(h.settings),
		Options: []getter.Option{
			getter.WithInsecureSkipVerifyTLS(h.insecure),
		},
		RepositoryConfig: h.settings.RepositoryConfig,
		RepositoryCache:  h.settings.RepositoryCache,
	}

	for _, chart := range cfg.Mirror.Helm.Local {

		// find images associations with chart (default values)
		img, err := findImages(chart.Path, chart.ImagePaths...)
		if err != nil {
			return nil, err
		}

		images = append(images, img...)
	}

	for _, repo := range cfg.Mirror.Helm.Repositories {

		// Add repo to temp file
		if err := h.repoAdd(repo); err != nil {
			return nil, err
		}

		for _, chart := range repo.Charts {
			logrus.Infof("Pulling chart %s", chart.Name)
			// TODO: Do something with the returned verifications
			ref := fmt.Sprintf("%s/%s", repo.Name, chart.Name)
			dest := filepath.Join(h.Dir, config.SourceDir, config.HelmDir)
			path, _, err := c.DownloadTo(ref, chart.Version, dest)
			if err != nil {
				return nil, fmt.Errorf("error pulling chart %q: %v", ref, err)
			}

			// find images associations with chart (default values)
			img, err := findImages(path, chart.ImagePaths...)
			if err != nil {
				return nil, err
			}

			images = append(images, img...)
		}
	}

	// Image download planning
	additional := NewAdditionalOptions(h.MirrorOptions)
	return additional.Plan(ctx, images)
}

// FindImages will download images found in a Helm chart on disk
func findImages(path string, imagePaths ...string) (images []v1alpha2.Image, err error) {

	logrus.Debugf("Reading from path %s", path)

	// Get all json paths where images
	// are located
	p := getImagesPath(imagePaths...)

	chart, err := loader.Load(path)
	if err != nil {
		return nil, err
	}

	manifest, err := render(chart)
	if err != nil {
		return nil, err
	}

	// Process each YAML document seperately
	for _, single := range bytes.Split([]byte(manifest), []byte("\n---\n")) {

		imgs, err := search(single, p...)

		if err != nil {
			return nil, err
		}

		images = append(images, imgs...)
	}

	return images, nil
}

// getImagesPath returns known jsonpaths and user defined
// json paths where images are found
func getImagesPath(paths ...string) []string {
	pathlist := []string{
		"{.spec.template.spec.initContainers[*].image}",
		"{.spec.template.spec.containers[*].image}",
		"{.spec.initContainers[*].image}",
		"{.spec.containers[*].image}",
	}
	return append(pathlist, paths...)
}

// render will return a templated chart
// TODO: add input for client.APIVersion
func render(chart *helmchart.Chart) (string, error) {

	// Client setup
	cfg := new(action.Configuration)
	client := action.NewInstall(cfg)
	client.DryRun = true
	client.ReleaseName = "RELEASE-NAME"
	client.Replace = true
	client.ClientOnly = true
	client.IncludeCRDs = true

	// Create empty extra values options
	valueOpts := make(map[string]interface{})

	// Run a relase dry run to get the manifest
	rel, err := client.Run(chart, valueOpts)

	return rel.Manifest, err
}

// repoAdd adds a Helm repo with given name and url
func (h *HelmOptions) repoAdd(chartRepo v1alpha2.Repository) error {

	entry := helmrepo.Entry{
		Name: chartRepo.Name,
		URL:  chartRepo.URL,
	}

	b, err := ioutil.ReadFile(h.settings.RepositoryConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var helmFile helmrepo.File
	if err := yaml.Unmarshal(b, &helmFile); err != nil {
		return err
	}

	// Check for existing repo name
	if helmFile.Has(chartRepo.Name) {
		logrus.Infof("repository name (%s) already exists", chartRepo.Name)
		return nil
	}

	// Check that the provided repo info is valid
	r, err := helmrepo.NewChartRepository(&entry, getter.All(h.settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return fmt.Errorf("invalid chart repository %q: %v", chartRepo.URL, err)
	}

	// Update temp file with chart entry
	helmFile.Update(&entry)

	if err := helmFile.WriteFile(h.settings.RepositoryConfig, 0644); err != nil {
		return fmt.Errorf("error writing helm repo file: %v", err)
	}

	return nil
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

// search will return images from parsed object
func search(yamlData []byte, paths ...string) (images []v1alpha2.Image, err error) {

	var data interface{}
	// yaml.Unmarshal will convert YAMl to JSON first
	if err := yaml.Unmarshal(yamlData, &data); err != nil {
		logrus.Error(err)
	}

	j := jsonpath.New("")
	j.AllowMissingKeys(true)

	for _, path := range paths {
		results, err := parseJSONPath(data, j, path)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			logrus.Debugf("Found image %s", result)
			img := v1alpha2.Image{
				Name: result,
			}

			images = append(images, img)
		}
	}

	return images, nil
}

// mkTempFile will make a temporary file and return the name
// and cleanup method
func mktempFile(dir string) (func(), string, error) {
	file, err := ioutil.TempFile(dir, "repo.*")
	return func() {
		if err := os.Remove(file.Name()); err != nil {
			logrus.Fatal(err)
		}
	}, file.Name(), err
}
