package generate

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Generator discovers what a registry holds.
type Generator struct {
	Source Source
	// Host is the registry host[:port], used to build image references.
	Host string
	// Prefix, when set, restricts discovery to repositories starting with it.
	Prefix string
}

// Discover classifies every repository as release, operator catalog or additional image.
func (g *Generator) Discover(ctx context.Context) (Content, error) {
	var c Content
	repos, err := g.Source.Repositories(ctx)
	if err != nil {
		return c, err
	}
	slices.Sort(repos)

	var releaseTags []string
	skippedDigests := 0
	for _, repo := range repos {
		if !g.matchesPrefix(repo) {
			continue
		}
		if g.Prefix != "" && repo == strings.TrimSuffix(g.Prefix, "/") && !isRepo(repo, releaseImagesRepo) {
			c.Warnings = append(c.Warnings, fmt.Sprintf("repository %q skipped because it equals the prefix; an empty target path cannot be represented", repo))
			continue
		}
		if isRepo(repo, releaseContentRepo) {
			// release payload components are implied by release-images
			continue
		}
		tags, err := g.Source.Tags(ctx, repo)
		if err != nil {
			return c, fmt.Errorf("listing tags for %q: %w", repo, err)
		}
		if isRepo(repo, releaseImagesRepo) {
			releaseTags = append(releaseTags, tags...)
			if root := strings.TrimSuffix(strings.TrimSuffix(repo, releaseImagesRepo), "/"); root != strings.TrimSuffix(g.Prefix, "/") {
				// The release collector appends openshift/release-images to the destination.
				c.Warnings = append(c.Warnings, fmt.Sprintf(
					"release repository %q is not at the destination root; mirror and delete releases against docker://%s%s", repo, g.Host, rootPath(root)))
			}
			continue
		}
		if err := g.classify(ctx, &c, repo, tags, &skippedDigests); err != nil {
			return c, err
		}
	}

	var warns []string
	c.Platform, warns = buildPlatform(releaseTags)
	c.Warnings = append(c.Warnings, warns...)
	if skippedDigests > 0 {
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"%d digest-derived tags skipped (operator related images, release components, Helm images); they cannot be told apart from digest-mirrored additional images", skippedDigests))
	}
	c.Warnings = append(c.Warnings, "image references point at the mirror registry, not the original sources; platform channels and operator related images inside rebuilt catalogs still resolve to upstream registries")
	return c, nil
}

// matchesPrefix reports whether repo is the Prefix path or lies under it.
// Matching is on path boundaries: prefix "team-a" does not match "team-ab/app".
func (g *Generator) matchesPrefix(repo string) bool {
	p := strings.TrimSuffix(g.Prefix, "/")
	return p == "" || repo == p || strings.HasPrefix(repo, p+"/")
}

// targetPath returns repo without the Prefix path, or "" when nothing is
// stripped. Mirroring to docker://host/<prefix> appends the target path to
// the prefix, so the prefix must not appear twice.
func (g *Generator) targetPath(repo string) string {
	p := strings.TrimSuffix(g.Prefix, "/")
	if p == "" || !strings.HasPrefix(repo, p+"/") {
		return ""
	}
	return strings.TrimPrefix(repo, p+"/")
}

// rootPath returns root as a URL path suffix: "" or "/root".
func rootPath(root string) string {
	if root == "" {
		return ""
	}
	return "/" + root
}

// classify adds every tag of repo to c, as an operator (one per catalog tag) or
// as an additional image. Each tag is probed on its own, so a repository can mix
// catalogs and ordinary images. A failed probe is recorded as a warning and the
// tag is treated as an additional image. Digest-derived tags are added to skipped.
func (g *Generator) classify(ctx context.Context, c *Content, repo string, tags []string, skipped *int) error {
	target := g.targetPath(repo)
	var images []string
	for _, tag := range slices.Sorted(slices.Values(tags)) {
		if isSignatureTag(tag) {
			continue
		}
		isCatalog, err := g.Source.IsCatalog(ctx, repo, tag)
		if err != nil {
			c.Warnings = append(c.Warnings, fmt.Sprintf(
				"could not tell whether %s:%s is an operator catalog, treating it as an additional image: %v", repo, tag, err))
		}
		if err != nil || !isCatalog {
			images = append(images, tag)
			continue
		}
		m, err := g.Source.CatalogModel(ctx, repo, tag)
		if err != nil {
			return fmt.Errorf("reading catalog %s:%s: %w", repo, tag, err)
		}
		op := buildOperator(fmt.Sprintf("%s/%s:%s", g.Host, repo, tag), m)
		op.TargetCatalog = target
		c.Operators = append(c.Operators, op)
	}
	imgs, n := buildAdditional(g.Host, repo, images)
	for i := range imgs {
		imgs[i].TargetRepo = target
	}
	c.AdditionalImages = append(c.AdditionalImages, imgs...)
	*skipped += n
	return nil
}
