package operator

import (
	"fmt"
	"sort"

	"github.com/blang/semver/v4"
	"github.com/imdario/mergo"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/alpha/property"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Merger is an object that will complete merge actions declarative config options
type Merger interface {
	Merge(*declcfg.DeclarativeConfig) error
}

func validate(cfg *declcfg.DeclarativeConfig) error {
	// configs are validated after being converted to a model
	_, err := declcfg.ConvertToModel(*cfg)
	return err
}

var _ Merger = &PreferLastStrategy{}

type PreferLastStrategy struct{}

func (mg *PreferLastStrategy) Merge(dc *declcfg.DeclarativeConfig) error {
	return mergeDCPreferLast(dc)
}

// mergeDCPreferLast merges all packages, channels, and bundles with the same unique key
// into single objects using the last element with that key.
func mergeDCPreferLast(cfg *declcfg.DeclarativeConfig) error {

	// Merge declcfg.Packages.
	pkgsByKey := make(map[string][]declcfg.Package, len(cfg.Packages))
	for i, pkg := range cfg.Packages {
		key := keyForDCObj(pkg)
		pkgsByKey[key] = append(pkgsByKey[key], cfg.Packages[i])
	}
	if len(pkgsByKey) != 0 {
		outPkgs := make([]declcfg.Package, len(pkgsByKey))
		i := 0
		for _, pkgs := range pkgsByKey {
			outPkgs[i] = pkgs[len(pkgs)-1]
			i++
		}
		sortPackages(outPkgs)
		cfg.Packages = outPkgs
	}

	// Merge channels.
	chsByKey := make(map[string][]declcfg.Channel, len(cfg.Channels))
	for i, ch := range cfg.Channels {
		key := keyForDCObj(ch)
		chsByKey[key] = append(chsByKey[key], cfg.Channels[i])
	}
	if len(chsByKey) != 0 {
		outChs := make([]declcfg.Channel, len(chsByKey))
		i := 0
		for _, chs := range chsByKey {
			outChs[i] = chs[len(chs)-1]
			i++
		}
		sortChannels(outChs)
		cfg.Channels = outChs
	}

	// Merge bundles.
	bundlesByKey := make(map[string][]declcfg.Bundle, len(cfg.Bundles))
	for i, b := range cfg.Bundles {
		key := keyForDCObj(b)
		bundlesByKey[key] = append(bundlesByKey[key], cfg.Bundles[i])
	}
	if len(bundlesByKey) != 0 {
		outBundles := make([]declcfg.Bundle, len(bundlesByKey))
		i := 0
		for _, bundles := range bundlesByKey {
			outBundles[i] = bundles[len(bundles)-1]
			i++
		}
		sortBundles(outBundles)
		cfg.Bundles = outBundles
	}

	// There is no way to merge "other" schema since a unique key field is unknown.
	return validate(cfg)
}

var _ Merger = &TwoWayStrategy{}

type TwoWayStrategy struct{}

func (mg *TwoWayStrategy) Merge(dc *declcfg.DeclarativeConfig) error {
	return mergeDCTwoWay(dc)
}

// mergeDCTwoWay merges all declcfg.Packages, channels, and bundles with the same unique key
// into single objects with ascending priority.
func mergeDCTwoWay(cfg *declcfg.DeclarativeConfig) error {
	var err error
	if cfg.Packages, err = mergePackages(cfg.Packages); err != nil {
		return err
	}

	if cfg.Channels, err = mergeChannels(cfg.Channels); err != nil {
		return err
	}

	if cfg.Bundles, err = mergeBundles(cfg.Bundles); err != nil {
		return err
	}

	model, err := convertToModelNoValidate(*cfg)
	if err != nil {
		return err
	}

	newCfg, err := resolveChannelHeads(model)
	if err != nil {
		return fmt.Errorf("error resolving channel heads: %v", err)
	}
	cfg.Bundles = newCfg.Bundles
	cfg.Channels = newCfg.Channels
	cfg.Others = newCfg.Others
	cfg.Packages = newCfg.Packages

	// There is no way to merge "other" schema since a unique key field is unknown.
	return validate(cfg)
}

// mergePackage Packages merges all declcfg.Packages with the same name into one declcfg.Package object.
// Value preference is ascending: values of declcfg.Packages later in input are preferred.
func mergePackages(inPkgs []declcfg.Package) (outPkgs []declcfg.Package, err error) {
	pkgsByName := make(map[string][]declcfg.Package, len(inPkgs))
	for i, pkg := range inPkgs {
		key := keyForDCObj(pkg)
		pkgsByName[key] = append(pkgsByName[key], inPkgs[i])
	}

	for _, pkgs := range pkgsByName {
		mergedPkg := pkgs[0]

		if len(pkgs) > 1 {
			for _, pkg := range pkgs[1:] {
				if err := mergo.Merge(&mergedPkg, pkg, mergo.WithOverride); err != nil {
					return nil, err
				}
			}
		}

		outPkgs = append(outPkgs, mergedPkg)
	}

	sortPackages(outPkgs)

	return outPkgs, nil
}

// mergeChannels merges all channels with the same name and declcfg.Package into one channel object.
// Value preference is ascending: values of channels later in input are preferred.
func mergeChannels(inChs []declcfg.Channel) (outChs []declcfg.Channel, err error) {
	chsByKey := make(map[string][]declcfg.Channel, len(inChs))
	entriesByKey := make(map[string]map[string][]declcfg.ChannelEntry, len(inChs))
	for i, ch := range inChs {
		chKey := keyForDCObj(ch)
		chsByKey[chKey] = append(chsByKey[chKey], inChs[i])
		entries, ok := entriesByKey[chKey]
		if !ok {
			entries = make(map[string][]declcfg.ChannelEntry)
			entriesByKey[chKey] = entries
		}
		for j, e := range ch.Entries {
			entries[e.Name] = append(entries[e.Name], ch.Entries[j])
		}
	}

	for chKey, chs := range chsByKey {
		mergedCh := chs[0]

		if len(chs) > 1 {
			for _, ch := range chs[1:] {
				if err := mergo.Merge(&mergedCh, ch, mergo.WithOverride); err != nil {
					return nil, err
				}
			}
		}

		mergedCh.Entries = nil
		for _, entries := range entriesByKey[chKey] {
			mergedEntry := entries[0]

			if len(entries) > 1 {
				for _, e := range entries[1:] {
					if err := mergo.Merge(&mergedEntry, e, mergo.WithOverride); err != nil {
						return nil, err
					}
				}
			}

			mergedCh.Entries = append(mergedCh.Entries, mergedEntry)
		}

		sort.Slice(mergedCh.Entries, func(i, j int) bool {
			return mergedCh.Entries[i].Name < mergedCh.Entries[j].Name
		})

		outChs = append(outChs, mergedCh)
	}

	sortChannels(outChs)

	return outChs, nil
}

// mergeBundles merges all bundles with the same name and declcfg.Package into one bundle object.
// Value preference is ascending: values of bundles later in input are preferred.
func mergeBundles(inBundles []declcfg.Bundle) (outBundles []declcfg.Bundle, err error) {
	bundlesByKey := make(map[string][]declcfg.Bundle, len(inBundles))
	for i, bundle := range inBundles {
		key := keyForDCObj(bundle)
		bundlesByKey[key] = append(bundlesByKey[key], inBundles[i])
	}

	for _, bundles := range bundlesByKey {
		mergedBundle := bundles[0]

		if len(bundles) > 1 {
			for _, bundle := range bundles[1:] {
				if err := mergo.Merge(&mergedBundle, bundle, mergo.WithOverride); err != nil {
					return nil, err
				}
			}
		}

		outBundles = append(outBundles, mergedBundle)
	}

	sortBundles(outBundles)

	return outBundles, nil
}

func sortPackages(pkgs []declcfg.Package) {
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
}

func sortChannels(chs []declcfg.Channel) {
	sort.Slice(chs, func(i, j int) bool {
		if chs[i].Package == chs[j].Package {
			return chs[i].Name < chs[j].Name
		}
		return chs[i].Package < chs[j].Package
	})
}

func sortBundles(bundles []declcfg.Bundle) {
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Package == bundles[j].Package {
			return bundles[i].Name < bundles[j].Name
		}
		return bundles[i].Package < bundles[j].Package
	})
}

func keyForDCObj(obj interface{}) string {
	switch t := obj.(type) {
	case declcfg.Package:
		// declcfg.Package name is globally unique.
		return t.Name
	case declcfg.Channel:
		// Channel name is unqiue per declcfg.Package.
		return t.Package + t.Name
	case declcfg.Bundle:
		// Bundle name is unqiue per declcfg.Package.
		return t.Package + t.Name
	default:
		// This should never happen.
		panic(fmt.Sprintf("bug: unrecognized type %T, expected one of declcfg.Package, Channel, Bundle", t))
	}
}

func resolveChannelHeads(inputModel model.Model) (declcfg.DeclarativeConfig, error) {
	for _, mpkg := range inputModel {
		for _, mch := range mpkg.Channels {
			incoming := map[string]int{}
			for _, b := range mch.Bundles {
				if b.Replaces != "" {
					incoming[b.Replaces]++
				}
				for _, skip := range b.Skips {
					incoming[skip]++
				}
			}
			heads := make(map[string]*model.Bundle)
			for _, b := range mch.Bundles {
				if _, ok := incoming[b.Name]; !ok {
					heads[b.Version.String()] = b
				}
			}

			if len(heads) > 1 {
				keys := make([]semver.Version, 0, len(heads))
				for k := range heads {
					keys = append(keys, semver.MustParse(k))
				}
				sort.Slice(keys, func(i, j int) bool {
					return keys[i].LT(keys[j])
				})
				head := heads[keys[len(keys)-1].String()]
				oldVers := keys[:len(keys)-1]
				for _, ver := range oldVers {
					if head.Replaces == ver.String() {
						continue
					}
					// If the old version falls within the new channel head's
					// skip range, prune the old head
					skipRange, err := semver.ParseRange(head.SkipRange)
					if err != nil {
						return declcfg.DeclarativeConfig{}, err
					}
					if skipRange(ver) {
						oldHead := heads[ver.String()]
						delete(mch.Bundles, oldHead.Name)
					}
				}

			}
		}
	}
	return declcfg.ConvertFromModel(inputModel), nil
}

// Copied from  https://github.com/operator-framework/operator-registry/blob/master/alpha/declcfg/declcfg_to_model.go
// Temporary solution
func convertToModelNoValidate(cfg declcfg.DeclarativeConfig) (model.Model, error) {
	mpkgs := model.Model{}
	defaultChannels := map[string]string{}
	for _, p := range cfg.Packages {
		if p.Name == "" {
			return nil, fmt.Errorf("config contains package with no name")
		}

		if _, ok := mpkgs[p.Name]; ok {
			return nil, fmt.Errorf("duplicate package %q", p.Name)
		}

		mpkg := &model.Package{
			Name:        p.Name,
			Description: p.Description,
			Channels:    map[string]*model.Channel{},
		}
		if p.Icon != nil {
			mpkg.Icon = &model.Icon{
				Data:      p.Icon.Data,
				MediaType: p.Icon.MediaType,
			}
		}
		defaultChannels[p.Name] = p.DefaultChannel
		mpkgs[p.Name] = mpkg
	}

	channelDefinedEntries := map[string]sets.String{}
	for _, c := range cfg.Channels {
		mpkg, ok := mpkgs[c.Package]
		if !ok {
			return nil, fmt.Errorf("unknown package %q for channel %q", c.Package, c.Name)
		}

		if c.Name == "" {
			return nil, fmt.Errorf("package %q contains channel with no name", c.Package)
		}

		if _, ok := mpkg.Channels[c.Name]; ok {
			return nil, fmt.Errorf("package %q has duplicate channel %q", c.Package, c.Name)
		}

		mch := &model.Channel{
			Package: mpkg,
			Name:    c.Name,
			Bundles: map[string]*model.Bundle{},
		}

		cde := sets.NewString()
		for _, entry := range c.Entries {
			if _, ok := mch.Bundles[entry.Name]; ok {
				return nil, fmt.Errorf("invalid package %q, channel %q: duplicate entry %q", c.Package, c.Name, entry.Name)
			}
			cde = cde.Insert(entry.Name)
			mch.Bundles[entry.Name] = &model.Bundle{
				Package:   mpkg,
				Channel:   mch,
				Name:      entry.Name,
				Replaces:  entry.Replaces,
				Skips:     entry.Skips,
				SkipRange: entry.SkipRange,
			}
		}
		channelDefinedEntries[c.Package] = cde

		mpkg.Channels[c.Name] = mch

		defaultChannelName := defaultChannels[c.Package]
		if defaultChannelName == c.Name {
			mpkg.DefaultChannel = mch
		}
	}

	// packageBundles tracks the set of bundle names for each package
	// and is used to detect duplicate bundles.
	packageBundles := map[string]sets.String{}

	for _, b := range cfg.Bundles {
		if b.Package == "" {
			return nil, fmt.Errorf("package name must be set for bundle %q", b.Name)
		}
		mpkg, ok := mpkgs[b.Package]
		if !ok {
			return nil, fmt.Errorf("unknown package %q for bundle %q", b.Package, b.Name)
		}

		bundles, ok := packageBundles[b.Package]
		if !ok {
			bundles = sets.NewString()
		}
		if bundles.Has(b.Name) {
			return nil, fmt.Errorf("package %q has duplicate bundle %q", b.Package, b.Name)
		}
		bundles.Insert(b.Name)
		packageBundles[b.Package] = bundles

		props, err := property.Parse(b.Properties)
		if err != nil {
			return nil, fmt.Errorf("parse properties for bundle %q: %v", b.Name, err)
		}

		if len(props.Packages) != 1 {
			return nil, fmt.Errorf("package %q bundle %q must have exactly 1 %q property, found %d", b.Package, b.Name, property.TypePackage, len(props.Packages))
		}

		if b.Package != props.Packages[0].PackageName {
			return nil, fmt.Errorf("package %q does not match %q property %q", b.Package, property.TypePackage, props.Packages[0].PackageName)
		}

		// Parse version from the package property.
		rawVersion := props.Packages[0].Version
		ver, err := semver.Parse(rawVersion)
		if err != nil {
			return nil, fmt.Errorf("error parsing bundle %q version %q: %v", b.Name, rawVersion, err)
		}

		channelDefinedEntries[b.Package] = channelDefinedEntries[b.Package].Delete(b.Name)
		found := false
		for _, mch := range mpkg.Channels {
			if mb, ok := mch.Bundles[b.Name]; ok {
				found = true
				mb.Image = b.Image
				mb.Properties = b.Properties
				mb.RelatedImages = relatedImagesToModelRelatedImages(b.RelatedImages)
				mb.CsvJSON = b.CsvJSON
				mb.Objects = b.Objects
				mb.PropertiesP = props
				mb.Version = ver
			}
		}
		if !found {
			return nil, fmt.Errorf("package %q, bundle %q not found in any channel entries", b.Package, b.Name)
		}
	}

	for pkg, entries := range channelDefinedEntries {
		if entries.Len() > 0 {
			return nil, fmt.Errorf("no olm.bundle blobs found in package %q for olm.channel entries %s", pkg, entries.List())
		}
	}

	for _, mpkg := range mpkgs {
		defaultChannelName := defaultChannels[mpkg.Name]
		if defaultChannelName != "" && mpkg.DefaultChannel == nil {
			dch := &model.Channel{
				Package: mpkg,
				Name:    defaultChannelName,
				Bundles: map[string]*model.Bundle{},
			}
			mpkg.DefaultChannel = dch
			mpkg.Channels[dch.Name] = dch
		}
	}

	mpkgs.Normalize()
	return mpkgs, nil
}

func relatedImagesToModelRelatedImages(in []declcfg.RelatedImage) []model.RelatedImage {
	var out []model.RelatedImage
	for _, p := range in {
		out = append(out, model.RelatedImage{
			Name:  p.Name,
			Image: p.Image,
		})
	}
	return out
}
