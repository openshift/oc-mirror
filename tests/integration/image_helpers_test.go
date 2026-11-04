package integration_test

import (
	"archive/tar"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

const (
	testCatalogRepo  = "oc-mirror/oc-mirror-dev"
	testCatalogTag   = "test-catalog-20261103-2e526488d"
	multiCatalogTag  = "test-catalog-multi-20261103-2e526488d"
	multilayersTag   = "multilayers-20261103-2e526488d"
	happyPathCatalog = "quay.io/" + testCatalogRepo + ":" + testCatalogTag
	operandImage     = "quay.io/" + testCatalogRepo + "@sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
)

var happyPathCachedImages = map[string]string{
	"openshift/release-images:v0.0.1":           "quay.io/oc-mirror/release/test-release-index:v0.0.1",
	"openshift/release:v0.0.1-alpine":           "quay.io/oc-mirror/release/test-image:v0.0.1",
	testCatalogRepo + ":" + testCatalogTag:      happyPathCatalog,
	"oc-mirror/oc-mirror-dev:foo-bundle-v0.3.1": "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.1",
	"oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0": "quay.io/oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0",
	"oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0": "quay.io/oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0",
	"oc-mirror/oc-mirror-dev:sha256-1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58":       operandImage,
	"openshifttest/hello-openshift:sha256-61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27": "quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27",
	"stefanprodan/podinfo:5.0.0": "ghcr.io/stefanprodan/podinfo:5.0.0",
}

var tagLinkPattern = regexp.MustCompile(`^(.+)/_manifests/tags/([^/]+)/current/link$`)
var rebuiltCatalogTagPattern = regexp.MustCompile(`^[a-f0-9]{32,128}$`)

// resolvedSourceDigests is cleared before each test so mutable tags are not cached across tests.
var resolvedSourceDigests = map[string]string{}

// sourceDigestTransport may be overridden to match a runner's proxy settings.
// Tests overriding it must restore it with DeferCleanup.
var sourceDigestTransport = remote.DefaultTransport

// remoteDigest resolves each source tag once per test; digest references need no lookup.
func remoteDigest(imageRef string) string {
	GinkgoHelper()
	if digest, ok := resolvedSourceDigests[imageRef]; ok {
		return digest
	}
	ref, err := name.ParseReference(imageRef)
	Expect(err).NotTo(HaveOccurred())
	if digest, ok := ref.(name.Digest); ok {
		return digest.DigestStr()
	}
	desc, err := remote.Head(ref,
		remote.WithAuth(authn.Anonymous),
		remote.WithContext(ctx),
		remote.WithTransport(sourceDigestTransport),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to resolve %s", imageRef)
	digest := desc.Digest.String()
	resolvedSourceDigests[imageRef] = digest
	return digest
}

// expectImagesExactly checks copied repository:tag entries against their source digests.
// Each rebuiltCatalogs entry permits one generated hash-tagged catalog in that repository;
// repeat a repository for incremental runs that retain more than one rebuilt catalog.
// Signature tags are checked separately by the signature tests. This checks references,
// not blob completeness: incremental archives intentionally omit previously archived blobs.
// The supplied inventories are not modified.
func expectImagesExactly(location string, actual, expected map[string]string, rebuiltCatalogs ...string) {
	GinkgoHelper()
	actual = maps.Clone(actual)
	wantedCatalogs, foundCatalogs := map[string]int{}, map[string]int{}
	for _, repo := range rebuiltCatalogs {
		wantedCatalogs[repo]++
	}
	for repoTag, digest := range actual {
		if strings.HasSuffix(repoTag, ".sig") {
			delete(actual, repoTag)
			continue
		}
		if _, copied := expected[repoTag]; copied {
			continue
		}
		repo, tag, _ := strings.Cut(repoTag, ":")
		if wantedCatalogs[repo] > 0 && rebuiltCatalogTagPattern.MatchString(tag) {
			Expect(digest).To(MatchRegexp(`^sha256:[a-f0-9]{64}$`))
			foundCatalogs[repo]++
			delete(actual, repoTag)
		}
	}
	Expect(foundCatalogs).To(Equal(wantedCatalogs), "%s has unexpected rebuilt catalog tags", location)
	digests := gstruct.Keys{}
	for repoTag, source := range expected {
		digests[repoTag] = Not(BeEmpty())
		if source != "" {
			digests[repoTag] = Equal(remoteDigest(source))
		}
	}
	Expect(actual).To(gstruct.MatchAllKeys(digests), "%s has unexpected image references", location)
}

// expectRegistryContainsExactly checks the registry's tagged images against their source digests.
func expectRegistryContainsExactly(reg registry.Registry, expected map[string]string) {
	GinkgoHelper()
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())
	actual := map[string]string{}
	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		for _, tag := range tags {
			digest, err := reg.TagDigest(ctx, repo, tag)
			Expect(err).NotTo(HaveOccurred())
			actual[repo+":"+tag] = digest
		}
	}
	expectImagesExactly("registry", actual, expected)
}

func expectLocalCacheContainsExactly(cacheDir string, expected map[string]string, rebuiltCatalogs ...string) {
	GinkgoHelper()
	root := filepath.Join(cacheDir, cacheRepositoriesSubdir)
	actual := map[string]string{}
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if match := tagLinkPattern.FindStringSubmatch(filepath.ToSlash(rel)); match != nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			actual[match[1]+":"+match[2]] = strings.TrimSpace(string(data))
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred())
	expectImagesExactly("local cache", actual, expected, rebuiltCatalogs...)
}

func expectArchiveContainsExactly(workDir string, expected map[string]string, rebuiltCatalogs ...string) {
	GinkgoHelper()
	actual := map[string]string{}
	for _, archive := range findTarArchives(workDir) {
		f, err := os.Open(archive)
		Expect(err).NotTo(HaveOccurred())
		tr := tar.NewReader(f)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			rel, ok := strings.CutPrefix(hdr.Name, tarRepositoriesPath)
			if !ok {
				continue
			}
			if match := tagLinkPattern.FindStringSubmatch(rel); match != nil {
				data, err := io.ReadAll(tr)
				Expect(err).NotTo(HaveOccurred())
				actual[match[1]+":"+match[2]] = strings.TrimSpace(string(data))
			}
		}
		Expect(f.Close()).To(Succeed())
	}
	expectImagesExactly("tar archive", actual, expected, rebuiltCatalogs...)
}
