package generate

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// TestBuildOperator checks package and channel inclusion with inferred version bounds.
func TestBuildOperator(t *testing.T) {
	pkg := &model.Package{Name: "foo"}
	pkg.Channels = map[string]*model.Channel{
		"stable": {Name: "stable", Package: pkg, Bundles: map[string]*model.Bundle{
			"foo.v1.0.0": {Name: "foo.v1.0.0", Version: semver.MustParse("1.0.0")},
			"foo.v1.2.0": {Name: "foo.v1.2.0", Version: semver.MustParse("1.2.0")},
			"foo.v1.1.0": {Name: "foo.v1.1.0", Version: semver.MustParse("1.1.0")},
		}},
		"empty": {Name: "empty", Package: pkg, Bundles: map[string]*model.Bundle{}},
	}
	op := buildOperator("reg.example.com/redhat/redhat-operator-index:v4.16", model.Model{"foo": pkg})

	assert.Equal(t, "reg.example.com/redhat/redhat-operator-index:v4.16", op.Catalog)
	assert.False(t, op.Full, "Full cannot be combined with version bounds")
	assert.Equal(t, []v2alpha1.IncludePackage{{
		Name: "foo",
		Channels: []v2alpha1.IncludeChannel{
			{Name: "empty"},
			{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "1.0.0", MaxVersion: "1.2.0"}},
		},
	}}, op.Packages)
}
