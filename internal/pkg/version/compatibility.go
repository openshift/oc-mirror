package version

// SupportedOCPVersions lists the OpenShift Container Platform versions that
// oc-mirror is actively tested against via the periodic e2e job matrix
// defined in openshift/release
// (ci-operator/config/openshift/oc-mirror/*__periodics.yaml).
//
// This list is intentionally NOT a contiguous min/max range: Red Hat's
// Extended Update Support (EUS) program can keep specific even-numbered
// OCP releases supported well past when intermediate odd releases reach
// end-of-life, and per-customer EUS enrollment isn't something oc-mirror
// can detect (it often runs before a cluster even exists, in disconnected
// environments with no network access to query support status).
// See https://access.redhat.com/support/policy/updates/openshift for the
// official OCP lifecycle/support policy.
//
// Maintainers: update this list whenever the periodic job matrix in
// openshift/release changes (new OCP stream added/retired).
var SupportedOCPVersions = []string{
	"4.21",
	"4.22",
	"4.23",
	// 5.0 is OCP's renumbered successor release and is functionally
	// identical to 4.23 for oc-mirror's compatibility purposes.
	"5.0",
}
