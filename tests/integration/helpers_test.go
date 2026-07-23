package integration_test

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"gopkg.in/yaml.v3"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

const (
	// releasePublicKeyFile is the GPG public key filename in the keys directory.
	releasePublicKeyFile = "release-pk.asc"
	platformReleaseRepo  = "openshift/release"

	dirWorkingDir = "working-dir"

	// Working dir subfolders
	dirOperatorCatalog  = "operator-catalogs"
	dirHelm             = "helm"
	dirClusterResources = "cluster-resources"
	idmsFileName        = "idms-oc-mirror.yaml"

	// cacheRepositoriesSubdir is the path within the oc-mirror cache directory to the local cache repositories.
	cacheRepositoriesSubdir = "docker/registry/v2/repositories"

	// tarRepositoriesPath is the path prefix for OCI repositories inside a tar archive.
	tarRepositoriesPath = "docker/registry/v2/repositories/"
)

// TODO: Remove these structs once we move the integration tests into the oc-mirror repo
type ImageSetConfiguration struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Mirror     MirrorConfig `yaml:"mirror"`
}

type MirrorConfig struct {
	Platform         PlatformConfig   `yaml:"platform"`
	Helm             HelmConfig       `yaml:"helm"`
	AdditionalImages []ImageRef       `yaml:"additionalImages"`
	Operators        []OperatorConfig `yaml:"operators"`
}

type PlatformConfig struct {
	Graph   bool   `yaml:"graph"`
	Release string `yaml:"release"`
}

type HelmConfig struct {
	Repositories []HelmRepository `yaml:"repositories"`
}

type HelmRepository struct {
	Name   string      `yaml:"name"`
	URL    string      `yaml:"url"`
	Charts []HelmChart `yaml:"charts"`
}

type HelmChart struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type ImageRef struct {
	Name string `yaml:"name"`
}

type OperatorConfig struct {
	Catalog  string            `yaml:"catalog"`
	Packages []OperatorPackage `yaml:"packages"`
}

type OperatorPackage struct {
	Name     string            `yaml:"name"`
	Channels []OperatorChannel `yaml:"channels"`
}

type OperatorChannel struct {
	Name       string `yaml:"name"`
	MinVersion string `yaml:"minVersion"`
	MaxVersion string `yaml:"maxVersion"`
}

// expectOcMirrorCommandSuccess asserts that the oc-mirror command completed successfully.
func expectOcMirrorCommandSuccess(result *ocmirror.Result, err error) {
	Expect(err).NotTo(HaveOccurred())
	Expect(result.ExitCode).To(Equal(0), "oc-mirror failed:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
}

// expectOcMirrorCommandFailure asserts that the oc-mirror command failed with a non-zero exit code.
func expectOcMirrorCommandFailure(result *ocmirror.Result, err error) {
	Expect(err).NotTo(HaveOccurred())
	Expect(result.ExitCode).NotTo(Equal(0), "expected oc-mirror to fail but it succeeded:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
}

// expectOcMirrorExitCode asserts that oc-mirror exited with the given code and that its
// combined output contains every one of the provided substrings.
func expectOcMirrorExitCode(result *ocmirror.Result, err error, expectedCode int, expectedMessages ...string) {
	Expect(err).NotTo(HaveOccurred())
	GinkgoWriter.Printf("oc-mirror exited with code %d (expected %d)\n", result.ExitCode, expectedCode)
	Expect(result.ExitCode).To(Equal(expectedCode),
		"expected exit code %d but got %d\nstdout: %s\nstderr: %s",
		expectedCode, result.ExitCode, result.Stdout, result.Stderr)

	output := result.Stdout + result.Stderr
	for _, msg := range expectedMessages {
		Expect(output).To(ContainSubstring(msg),
			"expected output to contain %q\nstdout: %s\nstderr: %s",
			msg, result.Stdout, result.Stderr)
	}
}

// expectCorrectTarArchiveContents verifies that the tar archive contains the expected
// repositories for the given ISC.
func expectCorrectTarArchiveContents(isc string, workDir string) {
	cfg := parseImageSetConfig(isc)

	matches, err := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).NotTo(BeEmpty(), "no tar archive found")

	entries := listTarEntries(matches[0])
	Expect(entries).NotTo(BeEmpty(), "tar archive has no entries")

	for _, p := range collectExpectedTarPaths(cfg) {
		expectTarContainsPath(entries, p)
	}
}

// listTarEntries opens a tar file and returns all entry names.
func listTarEntries(tarPath string) []string {
	f, err := os.Open(tarPath)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		closeErr := f.Close()
		Expect(closeErr).NotTo(HaveOccurred())
	}()

	tr := tar.NewReader(f)
	var entries []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		entries = append(entries, hdr.Name)
	}
	return entries
}

// expectTarContainsPath verifies that at least one tar entry equals prefix or is
// nested inside it (i.e. the directory or any of its contents is present).
func expectTarContainsPath(entries []string, prefix string) {
	prefix = filepath.ToSlash(prefix)
	for _, entry := range entries {
		e := filepath.ToSlash(entry)
		if e == prefix || e == prefix+"/" || strings.HasPrefix(e, prefix+"/") {
			return
		}
	}
	Expect(false).To(BeTrue(), "tar archive is missing expected path: %s", prefix)
}

// collectExpectedTarPaths returns all paths expected to be present in the tar archive
// for a given ImageSetConfig. Each content type is checked at its actual location:
// OCI artifacts under docker/registry/v2/repositories/, helm charts as .tgz files
// under working-dir/helm/charts/, and operator catalogs under working-dir/operator-catalogs/.
func collectExpectedTarPaths(cfg ImageSetConfiguration) []string {
	var paths []string

	// Platform release OCI repository
	if cfg.Mirror.Platform.Release != "" {
		paths = append(paths, tarRepositoriesPath+platformReleaseRepo)
	}

	// Operator catalog OCI repositories and working-dir catalog directories
	for _, op := range cfg.Mirror.Operators {
		paths = append(paths, tarRepositoriesPath+extractRepositoryName(op.Catalog))
		paths = append(paths, filepath.Join(dirWorkingDir, dirOperatorCatalog, extractImageName(op.Catalog)))
	}

	// Additional images OCI repositories
	for _, img := range cfg.Mirror.AdditionalImages {
		paths = append(paths, tarRepositoriesPath+extractRepositoryName(img.Name))
	}

	// Helm charts stored as .tgz files (not OCI repositories)
	for _, helmRepo := range cfg.Mirror.Helm.Repositories {
		for _, chart := range helmRepo.Charts {
			paths = append(paths, filepath.Join(dirWorkingDir, dirHelm, "charts", chart.Name+"-"+chart.Version+".tgz"))
		}
	}

	return paths
}

// expectSuccessfulMirrorInRegistry verifies that all the content specified on a given ImageSetConfig has been successfully mirrored into a registry
func expectSuccessfulMirrorInRegistry(isc string, registry registry.Registry) {
	cfg := parseImageSetConfig(isc)
	expectedRepos := collectExpectedRepos(cfg)

	// TODO: We need to verify individual operator images, not only the catalog
	expectRepositoriesExist(registry, expectedRepos)
}

// expectRepositoriesExist verifies that each expected repository substring is found in the registry.
func expectRepositoriesExist(registry registry.Registry, expected []string) {
	repos, err := registry.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(repos).NotTo(BeEmpty(), "registry has no repositories")

	for _, exp := range expected {
		found := false
		for _, repo := range repos {
			if strings.Contains(repo, exp) {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "missing repository %q, got: %v", exp, repos)
	}
}

// expectNoSignaturesInRegistry verifies that no .sig tags exist in the registry.
func expectNoSignaturesInRegistry(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())

	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		for _, tag := range tags {
			Expect(tag).NotTo(HaveSuffix(".sig"),
				"repo %q has unexpected signature tag %q when --remove-signatures=true", repo, tag)
		}
	}
}

// expectNonCatalogSignaturesDeleted verifies that after delete with --delete-signatures,
// no .sig tags remain in repos that don't contain a catalog image.
// Catalog signatures are never deleted by design.
func expectNonCatalogSignaturesDeleted(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())

	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		hasCatalog := false
		hasSigTag := false
		for _, tag := range tags {
			if strings.HasSuffix(tag, ".sig") {
				hasSigTag = true
				continue
			}
			if !hasCatalog {
				isCatalog, err := reg.IsCatalog(ctx, repo, tag)
				Expect(err).NotTo(HaveOccurred())
				hasCatalog = isCatalog
			}
		}

		if hasSigTag {
			Expect(hasCatalog).To(BeTrue(),
				"repo %q has .sig tags remaining but contains no catalog image", repo)
		}
	}
}

// expectSignaturesInRegistry verifies that every non-catalog image in the registry
// has a corresponding .sig tag.
func expectSignaturesInRegistry(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())

	sigCount := 0
	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		tagSet := make(map[string]struct{})
		for _, t := range tags {
			tagSet[t] = struct{}{}
		}

		for _, tag := range tags {
			if strings.HasSuffix(tag, ".sig") {
				continue
			}

			ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", reg.Endpoint(), repo, tag), name.Insecure)
			Expect(err).NotTo(HaveOccurred())

			desc, err := remote.Get(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
			Expect(err).NotTo(HaveOccurred())

			sigTag := strings.Replace(desc.Digest.String(), ":", "-", 1) + ".sig"
			_, hasSig := tagSet[sigTag]
			Expect(hasSig).To(BeTrue(),
				"image %s/%s:%s (digest %s) has no signature tag %s", reg.Endpoint(), repo, tag, desc.Digest, sigTag)
			sigCount++
		}
	}
	Expect(sigCount).To(BeNumerically(">", 0), "no images with signatures found in registry")
}

// expectOnlySignatureTagsRemain verifies that after a delete without --delete-signatures,
// non-catalog repos only have .sig tags remaining, and at least one .sig tag exists.
func expectOnlySignatureTagsRemain(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())

	foundSigTag := false
	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		if len(tags) == 0 {
			continue
		}
		for _, tag := range tags {
			if strings.HasSuffix(tag, ".sig") {
				foundSigTag = true
				continue
			}
			isCatalog, err := reg.IsCatalog(ctx, repo, tag)
			Expect(err).NotTo(HaveOccurred())
			Expect(isCatalog).To(BeTrue(),
				"non-catalog repo %q has non-signature tag %q after delete", repo, tag)
		}
	}
	Expect(foundSigTag).To(BeTrue(), "expected at least one .sig tag to remain after delete")
}

func expectEmptyRegistry(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())

	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		if len(tags) == 0 {
			continue
		}
		isCatalog, err := reg.IsCatalog(ctx, repo, tags[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(isCatalog).To(BeTrue(), "non-catalog repo %q still has tags after delete", repo)
	}
}

// parseImageSetConfig gets the path of an ImageSetConfig YAML and returns a parsed ImageSetConfig
func parseImageSetConfig(isc string) ImageSetConfiguration {
	data, err := os.ReadFile(isc)
	Expect(err).NotTo(HaveOccurred())

	var cfg ImageSetConfiguration
	err = yaml.Unmarshal(data, &cfg)
	Expect(err).NotTo(HaveOccurred())
	return cfg
}

// collectExpectedRepositories collects all the repos from an ImageSetConfig
// And returns a slice to be verified later
func collectExpectedRepos(cfg ImageSetConfiguration) []string {
	var expected []string

	// Collect releases
	if cfg.Mirror.Platform.Release != "" {
		// The name of the repo for releases is hardcoded.
		expected = append(expected, platformReleaseRepo)
	}

	// Collect operator catalogs
	for _, op := range cfg.Mirror.Operators {
		expected = append(expected, extractRepositoryName(op.Catalog))
	}

	// Collect helm charts
	for _, helmRepo := range cfg.Mirror.Helm.Repositories {
		if len(helmRepo.Charts) > 0 {
			expected = append(expected, helmRepo.Name)
		}
	}

	// Collect additional images
	for _, img := range cfg.Mirror.AdditionalImages {
		expected = append(expected, extractRepositoryName(img.Name))
	}

	return expected
}

// extractRepositoryName parses an image reference and extracts the repository part.
// It removes the registry prefix and any tag/digest suffix.
// Examples:
//   - "quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest" -> "oc-mirror/oc-mirror-dev"
//   - "quay.io/openshifttest/hello-openshift@sha256:..." -> "openshifttest/hello-openshift"
func extractRepositoryName(imageRef string) string {
	ref, err := reference.ParseNormalizedNamed(imageRef)
	Expect(err).NotTo(HaveOccurred(), "failed to parse image ref: %s", imageRef)
	return reference.Path(ref)
}

// extractImageName parses an image reference and extracts just the image name (final component).
// It removes the registry prefix, organization/namespace, and any tag/digest suffix.
// Examples:
//   - "quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest" -> "oc-mirror-dev"
//   - "registry.redhat.io/redhat/redhat-operator-index:v4.17" -> "redhat-operator-index"
func extractImageName(imageRef string) string {
	ref, err := reference.ParseNormalizedNamed(imageRef)
	Expect(err).NotTo(HaveOccurred(), "failed to parse image ref: %s", imageRef)
	return path.Base(reference.Path(ref))
}

// defaultCacheDir returns the default oc-mirror cache directory (~/.oc-mirror/.cache),
// used when oc-mirror is run without --cache-dir.
func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	Expect(err).NotTo(HaveOccurred())
	return filepath.Join(home, ".oc-mirror", ".cache")
}

// expectSuccessfulMirrorInLocalCache verifies that all content specified in a given ImageSetConfig
// has been cached locally by walking the cache dir.
func expectSuccessfulMirrorInLocalCache(isc string, cacheDir string) {
	cfg := parseImageSetConfig(isc)
	expectedRepos := collectExpectedRepos(cfg)

	repos, err := listLocalCacheRepositories(cacheDir)
	Expect(err).NotTo(HaveOccurred())
	Expect(repos).NotTo(BeEmpty(), "local cache has no repositories")

	for _, exp := range expectedRepos {
		found := false
		for _, repo := range repos {
			if strings.Contains(repo, exp) {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "missing repository %q in local cache, got: %v", exp, repos)
	}
}

// listLocalCacheRepositories walks <cacheDir>/docker/registry/v2/repositories/ and
// returns repository paths by locating _manifests directories.
func listLocalCacheRepositories(cacheDir string) ([]string, error) {
	reposRoot := filepath.Join(cacheDir, cacheRepositoriesSubdir)

	var repos []string
	err := filepath.Walk(reposRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "_manifests" {
			repoPath, relErr := filepath.Rel(reposRoot, filepath.Dir(path))
			if relErr == nil {
				repos = append(repos, repoPath)
			}
		}
		return nil
	})
	return repos, err
}

// copyFile copies a single file from src to dst, preserving file permissions
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Get the source file's permissions
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, info.Mode().Perm())
}

// setupWorkDir creates a temporary working directory and copies signature files.
// Returns the working directory path.
func setupWorkDir() string {
	workDir, err := os.MkdirTemp("", "oc-mirror-test-*")
	Expect(err).NotTo(HaveOccurred())

	// Copy release signature files to working directory
	sigDir := filepath.Join(workDir, dirWorkingDir, "signatures")
	err = os.MkdirAll(sigDir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	// Copy all release signature files from keys/ (any file that isn't the public key or cosign key)
	entries, err := os.ReadDir(keysDir)
	Expect(err).NotTo(HaveOccurred())

	var sigFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != releasePublicKeyFile && !strings.HasPrefix(entry.Name(), "cosign") {
			sigFiles = append(sigFiles, entry.Name())
		}
	}
	Expect(sigFiles).NotTo(BeEmpty(), "no signature files found in keys/")

	for _, sigFile := range sigFiles {
		err = copyFile(filepath.Join(keysDir, sigFile), filepath.Join(sigDir, sigFile))
		Expect(err).NotTo(HaveOccurred())
	}

	return workDir
}

// expectValidDeleteImagesFiles verifies that delete phase 1 created the expected delete YAML files.
// When deleteId is empty, it checks for default-named files.
// When deleteId is set, it checks for files with the delete-id suffix and verifies the defaults do not exist.
func expectValidDeleteImagesFiles(workDir, deleteId string) {
	deleteDir := filepath.Join(workDir, "working-dir", "delete")

	if deleteId == "" {
		_, err := os.Stat(filepath.Join(deleteDir, "delete-images.yaml"))
		Expect(err).NotTo(HaveOccurred(), "default delete-images.yaml not found")

		_, err = os.Stat(filepath.Join(deleteDir, "delete-imageset-config.yaml"))
		Expect(err).NotTo(HaveOccurred(), "default delete-imageset-config.yaml not found")
		return
	}

	_, err := os.Stat(filepath.Join(deleteDir, "delete-images-"+deleteId+".yaml"))
	Expect(err).NotTo(HaveOccurred(), "delete-images yaml with delete-id %q not found", deleteId)

	_, err = os.Stat(filepath.Join(deleteDir, "delete-imageset-config-"+deleteId+".yaml"))
	Expect(err).NotTo(HaveOccurred(), "delete-imageset-config yaml with delete-id %q not found", deleteId)

	_, err = os.Stat(filepath.Join(deleteDir, "delete-images.yaml"))
	Expect(err).To(HaveOccurred(), "default delete-images.yaml should not exist when --delete-id is set")
}

// expectValidMappingFile verifies that the dry-run mapping.txt file was created, that every line
// follows the source=destination format, and that all expected repositories from the ISC are represented.
func expectValidMappingFile(workDir, iscPath string) {
	mappingPath := filepath.Join(workDir, dirWorkingDir, "dry-run", "mapping.txt")
	data, err := os.ReadFile(mappingPath)
	Expect(err).NotTo(HaveOccurred(), "mapping.txt not found at: %s", mappingPath)

	content := strings.TrimSpace(string(data))
	Expect(content).NotTo(BeEmpty(), "mapping.txt is empty")

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		Expect(parts).To(HaveLen(2), "mapping line does not follow source=destination format: %s", line)
		Expect(parts[0]).NotTo(BeEmpty(), "source is empty in mapping line: %s", line)
		Expect(parts[1]).NotTo(BeEmpty(), "destination is empty in mapping line: %s", line)
	}

	cfg := parseImageSetConfig(iscPath)
	expectedRepos := collectExpectedRepos(cfg)
	for _, repo := range expectedRepos {
		Expect(content).To(ContainSubstring(repo),
			"mapping.txt does not contain expected repository %q", repo)
	}
}

// expectNoTarArchive verifies that no tar archive was created in the working directory.
func expectNoTarArchive(workDir string) {
	matches, err := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).To(BeEmpty(), "expected no tar archive but found: %v", matches)
}

// expectNoRepositoriesInRegistry verifies that the registry contains no repositories at all.
func expectNoRepositoriesInRegistry(reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(repos).To(BeEmpty(), "expected registry to have no repositories, but found: %v", repos)
}

// expectOnlyAllowedExecutableFiles walks the working directory and verifies that only files
// under graph-preparation/ or filtered-catalog-image/ directories have executable permissions.
func expectOnlyAllowedExecutableFiles(workDir string) {
	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&0o111 == 0 {
			return nil
		}

		relPath, relErr := filepath.Rel(workDir, path)
		Expect(relErr).NotTo(HaveOccurred())

		normalized := "/" + filepath.ToSlash(relPath)
		inGraphPreparation := strings.Contains(normalized, "/graph-preparation/")
		inFilteredCatalog := strings.Contains(normalized, "/filtered-catalog-image/")
		Expect(inGraphPreparation || inFilteredCatalog).To(BeTrue(), "unexpected executable file: %s", relPath)

		return nil
	})
	Expect(err).NotTo(HaveOccurred())
}

// expectCatalogBundlesMatchISC verifies that the mirrored catalog contains only the bundles
// that exactly match the expected bundle set for each operator package.
func expectCatalogBundlesMatchISC(ctx context.Context, reg registry.Registry, iscPath string, expectedBundles map[string][]string) {
	cfg := parseImageSetConfig(iscPath)

	for _, op := range cfg.Mirror.Operators {
		configsDir := extractCatalogConfigs(ctx, reg, op.Catalog)

		for _, pkg := range op.Packages {
			bundles := loadCatalogBundles(ctx, configsDir, pkg.Name)
			Expect(bundles).NotTo(BeEmpty(),
				"no bundles found for package %q in catalog %s", pkg.Name, op.Catalog)

			expected, ok := expectedBundles[pkg.Name]
			Expect(ok).To(BeTrue(),
				"no expected bundles provided for package %q", pkg.Name)

			var actual []string
			for _, b := range bundles {
				actual = append(actual, b.Name)
			}
			Expect(actual).To(ConsistOf(expected),
				"mirrored bundles for package %q do not match expected set", pkg.Name)
		}

		os.RemoveAll(configsDir)
	}
}

// expectRebuiltTagMatchesDigest verifies that the digest-style tag (sha256-<hash>)
// assigned to a rebuilt catalog image matches the actual manifest digest.
func expectRebuiltTagMatchesDigest(ctx context.Context, reg registry.Registry, iscPath string) {
	cfg := parseImageSetConfig(iscPath)
	digestTagPattern := regexp.MustCompile(`^sha256-[a-f0-9]{64}$`)

	for _, op := range cfg.Mirror.Operators {
		repo := extractRepositoryName(op.Catalog)
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred(), "failed to list tags for %q", repo)
		Expect(tags).NotTo(BeEmpty(), "expected at least one mirrored tag for %q", repo)

		foundDigestStyleTag := false
		for _, tag := range tags {
			if !digestTagPattern.MatchString(tag) {
				continue
			}

			foundDigestStyleTag = true
			ref, err := name.NewTag(reg.Endpoint()+"/"+repo+":"+tag, name.Insecure)
			Expect(err).NotTo(HaveOccurred(), "failed to create reference for %s:%s", repo, tag)

			desc, err := remote.Get(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
			Expect(err).NotTo(HaveOccurred(), "failed to fetch manifest for %s:%s", repo, tag)

			expectedTag := "sha256-" + strings.TrimPrefix(desc.Digest.String(), "sha256:")
			Expect(tag).To(Equal(expectedTag),
				"rebuilt tag should match manifest digest for %s: expected %q from digest %q, got %q",
				repo, expectedTag, desc.Digest.String(), tag)
		}

		Expect(foundDigestStyleTag).To(BeTrue(),
			"expected at least one digest-style rebuilt tag for %q, got: %v", repo, tags)
	}
}

// extractCatalogConfigs pulls a catalog image from the registry and extracts
// the FBC config files to a temporary directory on disk. The caller is
// responsible for removing the returned directory when done.
func extractCatalogConfigs(ctx context.Context, reg registry.Registry, catalogRef string) string {
	repo := extractRepositoryName(catalogRef)
	tag := extractTag(catalogRef)
	ref, err := name.NewTag(reg.Endpoint()+"/"+repo+":"+tag, name.Insecure)
	Expect(err).NotTo(HaveOccurred())

	img, err := remote.Image(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
	Expect(err).NotTo(HaveOccurred())

	// Get the FBC configs path from the standard OLM label
	cf, err := img.ConfigFile()
	Expect(err).NotTo(HaveOccurred())
	configsPath, ok := cf.Config.Labels["operators.operatorframework.io.index.configs.v1"]
	Expect(ok).To(BeTrue(), "catalog image missing FBC configs label")

	destDir, err := os.MkdirTemp("", "catalog-configs-*")
	Expect(err).NotTo(HaveOccurred())

	// Flatten the image into a single tar stream and extract config files to disk
	rc := mutate.Extract(img)
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		Expect(err).NotTo(HaveOccurred())

		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		cleanPath := filepath.Clean("/" + hdr.Name)
		if !strings.HasPrefix(cleanPath, configsPath) {
			continue
		}

		// Preserve the relative path under configsPath
		relPath, err := filepath.Rel(configsPath, cleanPath)
		Expect(err).NotTo(HaveOccurred())

		outPath := filepath.Join(destDir, relPath)
		Expect(os.MkdirAll(filepath.Dir(outPath), 0o755)).To(Succeed())

		data, err := io.ReadAll(tr)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(outPath, data, 0o644)).To(Succeed())
	}

	return destDir
}

// loadCatalogBundles loads the FBC configs from the subdirectory
// and returns all bundles for the given operator package.
func loadCatalogBundles(ctx context.Context, configsDir, packageName string) []declcfg.Bundle {
	pkgDir := filepath.Join(configsDir, packageName)
	cfg, err := declcfg.LoadFS(ctx, os.DirFS(pkgDir))
	Expect(err).NotTo(HaveOccurred(), "failed to load FBC from %s", pkgDir)

	var bundles []declcfg.Bundle
	for _, b := range cfg.Bundles {
		if b.Package == packageName {
			bundles = append(bundles, b)
		}
	}
	return bundles
}

func loadOtherBlobs(ctx context.Context, configsDir, packageName string) []declcfg.Meta {
	pkgDir := filepath.Join(configsDir, packageName)
	cfg, err := declcfg.LoadFS(ctx, os.DirFS(pkgDir))
	Expect(err).NotTo(HaveOccurred(), "failed to load FBC from %s", pkgDir)
	var others []declcfg.Meta
	for _, other := range cfg.Others {
		if other.Package == packageName {
			others = append(others, other)
		}
	}

	return others
}

// extractTag extracts the tag from an image reference (e.g., "quay.io/foo/bar:v4.19" -> "v4.19").
func extractTag(imageRef string) string {
	ref, err := reference.ParseNormalizedNamed(imageRef)
	Expect(err).NotTo(HaveOccurred(), "failed to parse image ref: %s", imageRef)
	tagged, ok := ref.(reference.Tagged)
	Expect(ok).To(BeTrue(), "image ref %s has no tag", imageRef)
	return tagged.Tag()
}

// cleanupWorkDir removes the working directory.
func cleanupWorkDir(workDir string) {
	if workDir != "" {
		os.RemoveAll(workDir)
	}
}

// expectCorrectIDMS reads the IDMS file generated by oc-mirror and verifies that:
// 1. Every YAML document has kind ImageDigestMirrorSet
// 2. No document contains any empty fields (empty strings, empty maps, empty slices, or nil values)
// 3. The IDMS sources cover all expected repositories from the ISC
func expectCorrectIDMS(workDir, iscPath string) {
	idmsPath := filepath.Join(workDir, dirWorkingDir, dirClusterResources, idmsFileName)

	data, err := os.ReadFile(idmsPath)
	Expect(err).NotTo(HaveOccurred(), "IDMS file not found at: %s", idmsPath)

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var allSources []string

	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		Expect(err).NotTo(HaveOccurred(), "failed to parse IDMS document")

		expectNoEmptyFields(raw, "idms")

		jsonBytes, err := json.Marshal(raw)
		Expect(err).NotTo(HaveOccurred(), "failed to marshal IDMS document to JSON")

		var idms configv1.ImageDigestMirrorSet
		Expect(json.Unmarshal(jsonBytes, &idms)).To(Succeed(), "failed to unmarshal IDMS into ImageDigestMirrorSet")

		Expect(idms.Kind).To(Equal("ImageDigestMirrorSet"))

		for _, mirror := range idms.Spec.ImageDigestMirrors {
			allSources = append(allSources, mirror.Source)
		}
	}

	expectIDMSContainsExpectedContent(iscPath, allSources)
}

// expectNoEmptyFields recursively walks a parsed YAML map and fails if any field
// has an empty value (empty string, empty map, empty slice, or nil).
func expectNoEmptyFields(obj map[string]any, path string) {
	for key, val := range obj {
		fieldPath := path + "." + key

		switch v := val.(type) {
		case string:
			Expect(v).NotTo(BeEmpty(), "field %s is an empty string", fieldPath)
		case map[string]any:
			Expect(v).NotTo(BeEmpty(), "field %s is an empty map", fieldPath)
			expectNoEmptyFields(v, fieldPath)
		case []any:
			Expect(v).NotTo(BeEmpty(), "field %s is an empty slice", fieldPath)
			for j, item := range v {
				itemPath := fmt.Sprintf("%s[%d]", fieldPath, j)
				if m, ok := item.(map[string]any); ok {
					expectNoEmptyFields(m, itemPath)
				}
				if s, ok := item.(string); ok {
					Expect(s).NotTo(BeEmpty(), "field %s is an empty string", itemPath)
				}
			}
		}
	}
}

// expectIDMSContainsExpectedContent verifies that all expected repositories from the ISC
// are present as sources in the IDMS.
func expectIDMSContainsExpectedContent(iscPath string, idmsSources []string) {
	cfg := parseImageSetConfig(iscPath)

	if cfg.Mirror.Platform.Release != "" {
		ref := referenceWithoutTagOrDigest(cfg.Mirror.Platform.Release)
		Expect(idmsSourceCoversRef(idmsSources, ref)).To(BeTrue(),
			"no IDMS source is a prefix of platform release %q; sources: %v", ref, idmsSources)
	}

	for _, op := range cfg.Mirror.Operators {
		ref := referenceWithoutTagOrDigest(op.Catalog)
		Expect(idmsSourceCoversRef(idmsSources, ref)).To(BeTrue(),
			"no IDMS source is a prefix of operator catalog %q; sources: %v", ref, idmsSources)
	}

	for _, img := range cfg.Mirror.AdditionalImages {
		ref := referenceWithoutTagOrDigest(img.Name)
		Expect(idmsSourceCoversRef(idmsSources, ref)).To(BeTrue(),
			"no IDMS source is a prefix of additional image %q; sources: %v", ref, idmsSources)
	}
}

// referenceWithoutTagOrDigest strips the tag or digest from an image reference,
// returning "domain/path" (e.g. "quay.io/ns/repo").
func referenceWithoutTagOrDigest(imageRef string) string {
	ref, err := reference.ParseNormalizedNamed(imageRef)
	Expect(err).NotTo(HaveOccurred(), "failed to parse image ref: %s", imageRef)
	return reference.Domain(ref) + "/" + reference.Path(ref)
}

// idmsSourceCoversRef returns true if any IDMS source is a prefix of the given image reference.
func idmsSourceCoversRef(sources []string, ref string) bool {
	for _, src := range sources {
		if strings.HasPrefix(ref, src) {
			return true
		}
	}
	return false
}

// logOcMirrorResult writes the oc-mirror command result to GinkgoWriter for diagnostics.
func logOcMirrorResult(label string, result *ocmirror.Result) {
	if result == nil {
		GinkgoWriter.Printf("[%s] result is nil\n", label)
		return
	}
	GinkgoWriter.Printf("[%s] exit_code=%d duration=%s\n", label, result.ExitCode, result.Duration)
	if result.Stdout != "" {
		GinkgoWriter.Printf("[%s] stdout:\n%s\n", label, result.Stdout)
	}
	if result.Stderr != "" {
		GinkgoWriter.Printf("[%s] stderr:\n%s\n", label, result.Stderr)
	}
}

// logRegistryRepositories lists all repositories in the registry and writes them to GinkgoWriter.
func logRegistryRepositories(label string, reg registry.Registry) {
	repos, err := reg.ListRepositories(ctx)
	if err != nil {
		GinkgoWriter.Printf("[%s registry] failed to list repositories: %v\n", label, err)
		return
	}
	GinkgoWriter.Printf("[%s registry] %d repositories:\n", label, len(repos))
	for _, repo := range repos {
		tags, tagErr := reg.ListTags(ctx, repo)
		if tagErr != nil {
			GinkgoWriter.Printf("  %s (tags error: %v)\n", repo, tagErr)
		} else {
			GinkgoWriter.Printf("  %s  tags=%v\n", repo, tags)
		}
	}
}

// isOCMirrorVersionBefore returns true if the OC_MIRROR_VERSION environment variable
// indicates a version strictly before major.minor. Returns false when the variable
// is unset or set to "main".
func isOCMirrorVersionBefore(major int, minor int) bool {
	ver := os.Getenv("OC_MIRROR_VERSION")
	if ver == "" || ver == "main" {
		return false
	}
	versionMatcher := regexp.MustCompile(`(\d)\.(\d+)`)
	parts := versionMatcher.FindStringSubmatch(ver)
	Expect(parts).ToNot(BeNil(), "invalid version format %q", ver)
	verMajor, err := strconv.Atoi(parts[1])
	Expect(err).ToNot(HaveOccurred(), "failed to parse oc mirror major version %q", ver)
	if verMajor == major {
		verMinor, err := strconv.Atoi(parts[2])
		Expect(err).ToNot(HaveOccurred(), "failed to parse oc mirror minor version %q", ver)
		return verMinor < minor
	}
	return verMajor < major
}
