# catalog-filter version-range validation

## Scope

Validate [the proposed catalog-filter fix](https://github.com/rdossant-rh/catalog-filter/tree/fix/version-range-rebuild-selection)
in oc-mirror, based on `origin/main` at `492318f9`.
The root module replaces catalog-filter v0.0.5 with the fork at immutable commit
`c3f61071c5b9c8baf71e92bb1ca05125e3aa6288`. No transitive dependencies change.
This replacement is for validation; an upstream release is still needed for a production dependency bump.

## Expected behavior

- [OCPBUGS-138615](https://redhat.atlassian.net/browse/OCPBUGS-138615): a pin to
  `1.12.1` selects all four semver-equivalent web-terminal rebuilds in the selected
  channel, rather than an arbitrary bundle from a package-wide Go map. A matching
  rebuild in an unselected channel must not be included. Every matching rebuild
  must survive, including whichever rebuild carries `olm.csv.metadata`.
- [OCPBUGS-61497](https://redhat.atlassian.net/browse/OCPBUGS-61497): a pin to
  Redis `7.4.6-6.1` must not include `7.8.6-1.1`. The tests also exercise the
  Keycloak prerelease/skip-chain scenario from the original upstream fix.

The proposed policy uses semver constraints, not exact version-string equality:
build metadata does not distinguish matches. Matching entries retain their
original upgrade edges and ordering. Out-of-range connectors can still be retained
with warnings if needed to produce a valid OLM channel. Thus an exact pin is not
an unconditional promise of one bundle or of no connectors for every possible graph.
An absent pinned version still fails, now with the package/channel/range-specific
empty-channel error instead of the removed `specificBundle` lookup error. The
operator unit test retains an exact error-message assertion for this new diagnostic.

Metadata is retained only on its original selected bundle. It is not recovered
from bundles excluded by `maxVersion`, copied between bundles, or used as a reason
to retain a newer out-of-range bundle.

## Regression coverage

`tests/integration/operator_version_pin_test.go` builds distinct tiny images and
synthetic valid catalogs in a local registry. The product bundle names and
versions model the reports; the upgrade graphs are synthetic, not extracted from
the authenticated Red Hat catalogs.

The table covers package-level and channel-level equal bounds in both
mirror-to-mirror and mirror-to-disk/disk-to-mirror workflows. Redis uses both
`production` and `7.4.6` channels. Disk import runs after removing the working
directory and cache and stopping the source registry.

Four web-terminal rows place typed `property.CSVMetadata` on each of the four
matching builds in turn, using operator-registry v1.74.0's `property.MustBuild`.
Exactly one matching build carries metadata in each row; excluded `1.13.0` also
carries distinct metadata. These are legitimate FBC properties on minimal bundle
fixtures, not full CSV manifests. Keycloak and Redis remain separate rows. The
six rows across two bound scopes and two workflows produce 24 specs. Selection
expectations are fixture-defined and independent of metadata and filter code.

Assertions inspect the destination catalog's exact packages, channel entries,
bundle metadata, and OLM model validity; verify selected image digests and readable
blobs; reject extra destination repositories and tags; and check generated
CatalogSource and ClusterCatalog resources.
An explicit bundle-name-to-property comparison also checks the exact metadata on
its original bundle and rejects metadata from the excluded newer bundle.

## Commands

The existing `bin/oc-mirror` was built with the pinned fork; test-only edits do
not require rebuilding. From `tests/integration`, run the following in three
separate processes, stopping on any failure (Ginkgo does not support using
`go test -count=3` for this repetition):

```sh
GOPROXY=off GOSUMDB=off go test -v -count=1 -timeout 30m . -ginkgo.focus "operator exact version selection"
```

The planned baseline comparison uses the already-built unchanged-main executable,
without changing the checkout's dependency files. It was not run because the
third fixed-binary run failed during registry setup (see below):

```sh
OC_MIRROR_BINARY=/tmp/opencode/oc-mirror-baseline/bin/oc-mirror GOPROXY=off GOSUMDB=off go test -v -count=1 -timeout 30m . -ginkgo.focus "operator exact version selection"
```

Integration-module vet command (executed):

```sh
GOPROXY=off GOSUMDB=off go vet ./...
```

Do not rerun the full remote suite in this sandbox or bypass its egress policy.

## Verification results

- `make tidy`: passed; only the catalog-filter checksum entries changed.
- `make build` and `./bin/oc-mirror --v2 --help`: passed. Build information
  confirms the executable uses the pinned fork revision.
- Initial focused CLI run: 12/12 passed.
- Updated metadata-focused runs, each a separate process using the exact command
  above (24 selected specs out of 136; 112 unrelated specs skipped):

  | Run | Passed | Failed | Duration | Log under `/tmp/opencode/` |
  | --- | --- | --- | --- | --- |
  | 1 | 24 | 0 | 332.001s | `catalog-filter-metadata-focused-pass1.log` |
  | 2 | 24 | 0 | 340.013s | `catalog-filter-metadata-focused-pass2.log` |
  | 3 | 22 | 2 | 739.337s | `catalog-filter-metadata-focused-pass3.log` |

  Run 1 used the original background job; runs 2 and 3 were sequential foreground
  calls with sufficient shell timeouts. Run 3 failed in the shared suite's
  `BeforeEach` at `integration_suite_test.go:86`, before either test body ran:
  `Registry not ready` / `registry not ready after 30s`. Affected rows were
  channel-level m2m with metadata on `1.12.1+0.1745393748.p`, and channel-level
  m2d+d2m Keycloak. The readiness-timeout cause remains unclassified; it is not
  established as an egress-policy failure or a filter regression. Testing stopped
  after this failed run; no retries or baseline run followed. Therefore three
  successful repetitions and executable baseline regression evidence are not
  available. No test was weakened and no registry/network policy was changed.
- Operator and image-builder package tests with `-race -count=1`: passed after
  updating the missing-version diagnostic assertion described above.
- Earlier `make vet`: passed. Updated integration module
  `GOPROXY=off GOSUMDB=off go vet ./...`: passed (log:
  `/tmp/opencode/catalog-filter-metadata-integration-vet.log`).
- `make format`: blocked by pre-existing alignment in unchanged
  `tests/integration/image_helpers_test.go`. The new test is gofmt-clean.
- `make verify`: blocked by that formatting prerequisite. Direct
  `golangci-lint run -c .golangci.yaml` also fails because the installed linter
  cannot decode the Go 1.27.3 standard library's export data (version 5 versus
  supported version 4).
- Changed Go files are gofmt-clean; scoped `git diff --check` passed.
- Previous full CLI suite (`go test -v -count=1 -timeout 30m ./...`):
  35 passed, 86 failed, 3 skipped (121/124 specs run). Sandbox egress denial of
  `https://cdn01.quay.io/` blob downloads is confirmed. Localhost registry timeouts
  also occurred and remain unclassified; not all failures can be attributed to
  the egress block. The full suite has not been rerun for this focused update.

Logs are kept under `/tmp/opencode`; only sanitized summaries are reported here,
without signed URL queries or credentials. The original authenticated product
catalogs were not mirrored; coverage uses the local reconstructions described
above. Full CSV-manifest handling and an actual OpenShift/OLM installation are
not covered.

At the later watcher-failure milestone, the operator reported
`Shell.NotFoundError` from the parent event-driven completion watcher and
`ENOSYS` from the earlier kernel pidfd helper. A single result/process check
confirmed that original Go PID `96995` and test PID `97184` no longer existed,
with no matching focused-test or CLI runtime active. The original log contained
the successful 24/24 terminal result above; the third run's terminal failures
were also confirmed. No process termination, repeated polling, security-policy
workaround, additional CLI run, or baseline comparison was needed or attempted.
These watcher/tooling limitations do not change the recorded test outcomes.

## Independent review

The operator reported that separate implementation and adversarial reviewer
sessions completed with no supported Major, Medium, or Minor findings. Both
reviewers independently inspected the current files and local main baseline.
These were static reviews, not executable baseline validation. Git execution
tooling was unavailable to the reviewers, so they could not independently verify
working-tree status classification. The implementing session separately checked
the exact scoped diff and Git status before finalization. No code was changed
after these reviews; only validation results and review status were documented.
