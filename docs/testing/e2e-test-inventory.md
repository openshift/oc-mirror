# E2E Test Inventory and Migration Plan

This document catalogs every e2e test in `tests/e2e/test/e2e/oc_mirror_v2.go` and classifies each as one of:

- **Integration** -- can be fully validated with the oc-mirror CLI and a local registry; should be migrated to the integration test suite.
- **E2E** -- requires an OpenShift cluster (e.g. operator installation, CatalogSource pod verification, ClusterCatalog/ClusterExtension creation).
- **Obsolete** -- tests oc-mirror v1 functionality; should be removed.

Priority reflects the importance of the feature being tested:

| Priority | Meaning |
|----------|---------|
| High | Core mirroring workflows (m2d, d2m, m2m) and operator/release mirroring |
| Medium | Important features used by most users (filtering, delete, multi-arch, helm, registries.conf, signatures) |
| Low | Edge cases, warnings, negative tests, supporting flags |

Integration coverage column values:

- **Covered** -- testing goals already met by existing integration tests; no migration needed, can retire from e2e.
- **Partial** -- some goals overlap with integration tests but the e2e test adds scenarios not yet covered.
- *(empty)* -- not covered by any existing integration test; migration needed.

---

## Test List

| # | Test ID | Title | Description | Disposition | Priority | Integration Coverage | Notes |
|---|---------|-------|-------------|-------------|----------|---------------------|-------|
| 1 | 72973 | Mirror multi-arch additional images | m2d + d2m of multi-arch additional images, validates multi-arch manifests in registry | Integration | Medium | Covered: `TestIntegrationAdditional` + sparse_manifest tests | |
| 2 | 73359 | Mirror2mirror for operator [Level0] | m2m for operator catalog, creates CatalogSource, installs operator from custom CS | E2E | High | | Requires cluster to install and verify operator |
| 3 | 73452 | Mirror2mirror for OCI operator and additional image | m2m for OCI operator catalog, creates CatalogSource, installs operator from custom CS | E2E | High | | Requires cluster to install and verify operator |
| 4 | 73377 | Dry-run for v2 | Validates dry-run mode for m2d, d2m, and m2m; checks mapping.txt source/dest correctness | Integration | Medium | Covered: `dry_run_test.go` (m2m, m2d+d2m, manifest list) | |
| 5 | 72949 | TargetCatalog and targetTag for m2m and m2d/d2m | Validates targetCatalog and targetTag settings for docker and OCI catalogs | Integration | Medium | | |
| 6 | 72938 | Invalid operator filter error messages | Validates clear error for invalid operator filter (mixed min/max, Full:true with versionRange) | Integration | Low | Partial: invalid version range covered by `exit_codes_test.go`. Missing: mixed channel/package min/max error, Full:true with versionRange error | |
| 7 | 72942/72918/72709 | Max-nested-paths for v2 | m2d + d2m with --max-nested-paths, validates IDMS/ITMS path structure | Integration | Medium | | |
| 8 | 72947/72948 | OCI filtering for v2 | m2m with OCI operator catalog and filtering | Integration | Medium | Partial: operator filtering logic covered by `operators_test.go`. Missing: OCI-local catalog as source path | |
| 9 | 72913 | Archive max size for v2 | Validates --strict-archive error and normal m2d with archiveSize config | Integration | Medium | | |
| 10 | 72972/73381/74519 | Architecture specification for payload | m2m with specific architectures, validates payload arch; includes delete workflow | Integration | Medium | Partial: platform filtering covered by `sparse_manifest_test.go`, delete by `delete_test.go`. Missing: combined arch specification for releases + delete in single flow | |
| 11 | 74649/74646 | EUS channel warning (minor >=2) for v2 | Validates EUS upgrade path warning for m2d, m2m, and dry-run modes | Integration | Low | | |
| 12 | 74650 | EUS no-warning (minor <2) for v1 | Tests v1 EUS no-warning behavior | Obsolete | - | | v1 test |
| 13 | 74660/74646 | EUS channel warning (minor >=2) for v1 | Tests v1 EUS warning behavior | Obsolete | - | | v1 test |
| 14 | 74733 | EUS no-warning (minor <2) for v2 | Validates no EUS warning when minor diff < 2 or patch-only diff | Integration | Low | | |
| 15 | 73783 | No IDMS/ITMS when nothing mirrored | m2m with invalid digest, verifies cluster-resources dir is empty | Integration | Low | | |
| 16 | 72971 | Mirror multiple catalogs (docker + OCI) | m2m with multiple catalog sources (certified, marketplace, OCI), installs operator | E2E | High | | Installs operators from multiple mirrored CatalogSources |
| 17 | 72917 | Docker operator catalog filtering | m2m with filtered docker operator catalog | Integration | High | Covered: `operators_test.go` (version range + pinned version; docker is the default source type) | |
| 18 | 73124 | Non-RHOI catalog structure | m2d + m2m for IBM catalog with different index structure, installs operator | Integration | Medium | | Core feature is non-standard catalog support; operator install is secondary |
| 19 | 72708 | Delete with force-cache-delete | m2d + d2m + delete --generate + delete --force-cache-delete=true | Integration | Medium | Covered: `delete_test.go` (--force-cache-delete=true validates registry empty + cache cleanup) | |
| 20 | 72983 | registries.conf for operator mirror | m2m using registries.conf to remap sources across two registries | Integration | Medium | Covered: `enclave_test.go` (registries.conf redirect for operators between registries) | |
| 21 | 72982 | registries.conf for OCI | m2m with OCI catalog using registries.conf across two registries | Integration | Medium | Partial: registries.conf redirect covered by `enclave_test.go`. Missing: OCI-local catalog with registries.conf | |
| 22 | 75425 | KubeVirt CoreOS container image mirroring | m2d + d2m + m2m with kubeVirtContainer=true, validates CLI output message | E2E | Medium | | Requires real release images; kubeVirt validation will be folded into an existing release-mirroring E2E test |
| 23 | 75437 | kubeVirtContainer=false no error | m2d + d2m + m2m with kubeVirtContainer=false, verifies no error and kubeVirt image not included | Integration | Medium | | |
| 24 | 75438 | kubeVirtContainer=true for release without image | m2d + d2m + m2m with kubeVirtContainer=true for a release that lacks the image | Integration | Medium | | |
| 25 | 72920 | Head-only for catalog | m2d + d2m + m2m for head-only operator catalog | Integration | Medium | Partial: pinned version (single bundle) covered by `operators_test.go`. Missing: explicit head-only config keyword | |
| 26 | 75422 | Skip deletion of catalog image in delete | m2d + d2m + delete --generate, validates catalog index images are not in delete-images.yaml | Integration | Medium | | |
| 27 | 73791 | BlockedImages feature | m2d + d2m + m2m with blockedImages configuration | Integration | Medium | | |
| 28 | 76469 | Release signature configmap creation | m2d + d2m + m2m, validates signature configmap content matches signature directory | Integration | Medium | | |
| 29 | 76596 | No signature configmap without release images | m2d + d2m + m2m with operators-only ISC, verifies no signature configmap generated | Integration | Medium | | |
| 30 | 76489 | Cincinnati API error handling | Validates error message when UPDATE_URL_OVERRIDE points to invalid host | Integration | Low | Covered: `exit_codes_test.go` (exit code 2 for nonexistent release channel covers Cincinnati failure) | |
| 31 | 76597 | Delete with --generate | m2d + d2m + delete --generate, validates delete-images.yaml is created | Integration | Medium | Covered: `delete_test.go` (delete workflow uses --generate and validates file creation) | |
| 32 | 77060 | Helm chart mirroring | m2d + d2m + m2m for helm charts, validates IDMS/ITMS generation | Integration | Medium | | |
| 33 | 77061 | Delete helm images | m2d + d2m + delete helm images (without force-cache-delete), then m2m + delete | Integration | Medium | | |
| 34 | 77693 | Delete helm with force-cache-delete | m2d + d2m + delete --force-cache-delete=true for helm, then m2m + delete | Integration | Medium | | |
| 35 | 79217 | Delete v1 OCI operator images with v2 | Mirrors OCI operators with v1, then uses v2 --delete-v1-images to delete them | Obsolete | - | | v1 interop test |
| 36 | 79215 | ClusterCatalog creation with OLM v1 | m2m, creates CatalogSource + ClusterCatalog, creates ClusterExtension, installs operator via OLM v1 | E2E | High | | Requires cluster with OLM v1 for ClusterCatalog/ClusterExtension |
| 37 | 79452 | Delete v1 additional images with v2 | Mirrors additional images with v1, then uses v2 --delete-v1-images to delete them | Obsolete | - | | v1 interop test |
| 38 | 79408 | Cache-dir flag for m2d + d2m | Validates --cache-dir flag places cache in specified directory for m2d and d2m | Integration | Low | | |
| 39 | 79409 | Cache-dir flag for m2m | Validates --cache-dir flag places cache in specified directory for m2m | Integration | Low | | |
| 40 | 83582 | BlockedImages excludes images (dry-run) | m2d --dry-run, validates blocked images (aws, gcp, ibm, azure, openstack) absent from mapping.txt | Integration | Medium | | |
| 41 | 83849 | Helm operand images via env vars | m2d + d2m + m2m for helm chart with operand images referenced via environment variables | Integration | Medium | | |
| 42 | 83864 | Invalid helm package file errors | m2d with missing, empty, and invalid helm packages; validates error messages | Integration | Low | Partial: missing helm file covered by `exit_codes_test.go` (exit code 16). Missing: empty file and invalid content error messages | |
| 43 | 83875 | Verify credentials/hostname/certs before d2m | m2d + d2m with missing registry, wrong port, bad hostname, bad cert, then valid params | Integration | Medium | | |
| 44 | 84007 | OLM v1 operators concurrent mirroring | Concurrent m2m for list of operators, validates tags and repos in registry | Integration | High | Partial: single-operator filtering covered by `operators_test.go`. Missing: concurrent multi-operator mirroring at scale | |
| 45 | 86309 | Signature mirroring as default | m2m with operator, validates signatures are mirrored to registry by default | Integration | Medium | Covered: `signature_test.go` (signature preservation + deletion tests) | |
| 46 | 88132 | targetTag/targetRepo for m2d + d2m | Tests targetTag, targetRepo, combined, digest-with-tag, invalid targetRepo, and OCI for m2d + d2m | Integration | Medium | | |
| 47 | 88156 | targetTag/targetRepo for m2m | Same as 88132 but for m2m workflow | Integration | Medium | | |
| 48 | 87992 | Pinned catalog m2d + d2m | m2d generates pinned ISC/DISC with digests, second m2d + d2m using pinned ISC | Integration | High | | |
| 49 | 87962 | Operator images pinned by digest | m2d, m2m, and multi-catalog m2m; validates pinned ISC/DISC contain digests, no tags | Integration | High | | |

