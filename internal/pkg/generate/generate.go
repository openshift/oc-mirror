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
		if !strings.HasPrefix(repo, g.Prefix) {
			continue
		}
		tags, err := g.Source.Tags(ctx, repo)
		if err != nil {
			return c, fmt.Errorf("listing tags for %q: %w", repo, err)
		}
		switch {
		case isRepo(repo, releaseContentRepo):
			// release payload components are implied by release-images
		case isRepo(repo, releaseImagesRepo):
			releaseTags = append(releaseTags, tags...)
		default:
			if err := g.classify(ctx, &c, repo, tags, &skippedDigests); err != nil {
				return c, err
			}
		}
	}

	var warns []string
	c.Platform, warns = buildPlatform(releaseTags)
	c.Warnings = append(c.Warnings, warns...)
	if skippedDigests > 0 {
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"%d digest-derived tags skipped (operator related images, release components, Helm images); they cannot be told apart from digest-mirrored additional images", skippedDigests))
	}
	c.Warnings = append(c.Warnings, "image references point at the mirror registry, not the original sources")
	return c, nil
}

func (g *Generator) classify(ctx context.Context, c *Content, repo string, tags []string, skipped *int) error {
	var candidates []string
	for _, t := range slices.Sorted(slices.Values(tags)) {
		if !isSignatureTag(t) {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	isCatalog, err := g.Source.IsCatalog(ctx, repo, candidates[0])
	if err != nil {
		return fmt.Errorf("inspecting %s:%s: %w", repo, candidates[0], err)
	}
	if !isCatalog {
		imgs, n := buildAdditional(g.Host, repo, candidates)
		c.AdditionalImages = append(c.AdditionalImages, imgs...)
		*skipped += n
		return nil
	}
	for _, tag := range candidates {
		m, err := g.Source.CatalogModel(ctx, repo, tag)
		if err != nil {
			return fmt.Errorf("reading catalog %s:%s: %w", repo, tag, err)
		}
		c.Operators = append(c.Operators, buildOperator(fmt.Sprintf("%s/%s:%s", g.Host, repo, tag), m))
	}
	return nil
}
