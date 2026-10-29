# Delete workflow redesign (v2)

| | |
|---|---|
| **Status** | Proposal — under review |
| **Date** | 2026-10-29 |
| **Scope** | `oc-mirror delete` subcommand, mirror-run bookkeeping |
| **Target audience** | oc-mirror architects and developers |

This document proposes a redesign of the v2 delete workflow. It is intentionally
written before any implementation work so the team can review, iterate on, and
approve the design.

## Background: the delete workflow today

The delete functionality is a two-stage process managed by the `delete`
subcommand:

1. **Stage 1 — generate** (`oc-mirror delete --generate --config <DISC>
   --workspace file://<wd> docker://<reg>`): the `DeleteImageSetConfiguration`
   (DISC) is read and *coerced into an `ImageSetConfiguration`*
   (see `DeleteSchema.CompleteDelete` in `internal/pkg/cli/delete.go`). The full
   collector pipeline (`CollectAll`) is then re-run against the current state of
   the sources (upstream registries, Cincinnati, or their cached copies in the
   workspace), and the resulting image list is written to
   `<workspace>/working-dir/delete/delete-images[-<delete-id>].yaml`
   (a `DeleteImageList`, see `WriteDeleteMetaData` in
   `internal/pkg/delete/delete_images.go`).
2. **Stage 2 — execute** (`oc-mirror delete --delete-yaml-file <yaml>
   docker://<reg>`): the generated `DeleteImageList` is read back, each entry is
   validated to live under the destination registry, and image *manifests* are
   deleted from the target registry through the batch worker (DeleteMode).
   Optionally the local cache is deleted as well (`--force-cache-delete`).

Notable supporting facts about what is (and is not) persisted today:

- After every successful m2d and m2m run, oc-mirror writes
  `isc_pinned_<ts>.yaml` and `disc_pinned_<ts>.yaml` into the working-dir
  (`createConfigsWithPinnedCatalogs()` in `internal/pkg/cli/executor.go`). The
  ISC/DISC written there have operator catalogs pinned to digests (OCPBUGS-81712).
- The **history** (`internal/pkg/history`) tracks *blobs* for incremental
  archiving — not images, and not provenance.
- **No artifact persists the set of images that was actually mirrored.** The
  batch worker produces exactly this information (`copiedSchema`), but it is
  discarded after cluster-resources/archive generation.
- d2m writes no new metadata at all; it relies on the working-dir extracted
  from the archive.
- `ReadDeleteMetaData`'s "file does not exist" error message advises the user to
  "perform a delete with --dry-run" — a flag that does not exist for `delete`.

## Problems with the current design

The issues below are listed in no particular order; each one is addressed in the
proposed design.

1. **Two stages.** The `--generate` / execute split exists to give the user a
   reviewable artifact before a destructive operation, but it costs an extra
   invocation, extra flags (`--delete-id`, `--delete-yaml-file`), and a state
   file that can go stale between the two stages. The generate step could
   instead happen as part of a mirror run (m2m or d2m), when the image list is
   already known.

2. **Delete re-runs collection.** The generate stage derives its image list
   from a *fresh* collection driven by the DISC. Collection resolves image lists
   from the current state of the sources (upstream registries and Cincinnati, or
   their cached copies). If upstream content changed since the mirror — e.g. a
   catalog tag now resolves to a newer index, or channel heads have moved — the
   generated list diverges from what was actually mirrored:
   - images discovered by the new collection that were never mirrored are not
     found in the disconnected registry and are silently ignored;
   - images that *were* mirrored but no longer appear in the collection are
     never deleted and remain orphaned in the registry.

3. **The DISC is used in isolation.** A single central disconnected registry can
   serve several enclaves, and images can be shared between mirror runs in
   non-obvious ways (an operator bundle image that is also an
   `additionalImage`; a related image shared by two catalogs; images shared
   between two ISCs by different teams). The current delete has no visibility
   into any of this: whatever the DISC-driven collection produces gets deleted.

4. **A lost ISC means no delete.** If the user loses the original ISC, it cannot
   be converted into a DISC, and the only remaining option is to wipe the
   registry. The pinned ISC/DISC files written by recent versions mitigate this
   only if the workspace is intact *and* the mirror was made with a version that
   writes them.

5. **Pinning is not leveraged.** Pinned ISC/DISC (catalogs resolved to digests
   at mirror time) already exist and give exact, reproducible image sets. The
   delete workflow does not build on this concept.

6. **Inconsistent missing-image behavior.** The batch worker treats all errors
   alike; a `manifest unknown` for an image that was already deleted is
   indistinguishable from any other failure. The current compromise is to
   ignore such cases and continue, which is poor UX: the user gets no signal
   that part of the delete was a no-op, and cannot tell "already deleted" apart
   from "never existed / wrong registry".

7. **Backwards compatibility.** Any new design must keep working against
   registries populated by older oc-mirror versions (including v1 content via
   `--delete-v1-images`).

## Design goals / non-goals

**Goals**

- Single-stage delete: one command, with an in-band review/confirmation step.
- The delete image list is derived from **what was actually mirrored**, never
  from a fresh collection.
- Dependency safety: shared images are detected and protected by default.
- Deleting without the original ISC must be possible.
- Consistent, explicit, reported behavior for images not found in the registry.
- Backwards compatible with existing workspaces and older mirrors.
- Works air-gapped, without upstream connectivity.

**Non-goals (for the first iteration)**

- Deleting blobs from the remote registry (remains the registry
  administrator's garbage-collection job).
- Cross-workspace / cross-machine coordination for shared registries (see
  [registry-side records](#open-questions) as a follow-up).
- Cleaning up generated cluster resources (CatalogSource, IDMS/ITMS) — see
  open questions.
- v1 pruning semantics.

## Proposed design

### The core idea

The root cause of most of the problems above is that delete *derives* its image
list by re-running collection against current source state. The fix is to make
the mirror itself persist what it actually mirrored — a **`MirrorRecord`** —
and to redefine delete as **set arithmetic over records**.

A `MirrorRecord` is written at the end of every successful mirror run (m2d,
m2m, d2m). It captures:

- the exact set of images that were successfully copied (the batch worker's
  `copiedSchema`), digest-resolved at mirror time;
- the provenance of each image (which catalog/package/channel/additional-image
  entry caused it to be mirrored);
- run metadata (run id, timestamp, mode, destination, oc-mirror version,
  ISC digest).

Everything else in the design follows from this artifact.

### The `MirrorRecord` artifact

#### Schema (sketch)

```yaml
kind: MirrorRecord
apiVersion: mirror.openshift.io/v2alpha1
metadata:
  runId: 3f2a9c1e-...            # UUID
  timestamp: 2026-10-29T10:00:00Z
  mode: m2d                       # m2d | m2m | d2m
  destination: docker://mirror.corp.local   # refcount scoping (see below)
  ocMirrorVersion: 4.19.0
  iscDigest: sha256:...           # digest of the pinned ISC used for the run
images:
  - origin: registry.redhat.io/rh-osbs/...
    source: docker://...
    destination: docker://mirror.corp.local/...@sha256:...
    type: OperatorBundle          # v2alpha1.ImageType, as today
    rebuiltTag: ""                # set for rebuilt (filtered) catalogs
    associations:                 # provenance, for reference counting
      - catalog: redhat-operator-index@sha256:...
        package: aws-load-balancer-operator
        channel: alpha
    signatures:                   # signature tags captured at mirror time
      - sha256-...  (sigstore attachment tags)
deletions:                        # appended when a later delete removes images
  - timestamp: 2026-11-05T09:12:00Z
    images:
      - docker://mirror.corp.local/...@sha256:...
```

Design notes:

- **Only successfully copied images are recorded.** Partially failed runs
  produce a faithful record of what actually landed in the registry (or cache).
- **Associations are required** (issue 3): they are the record-side equivalent
  of today's `CopyImageSchemaMap` (`OperatorsByImage`, `BundlesByImage`), plus
  an explicit marker for `additionalImages` and helm entries. An image can (and
  legitimately will) appear once with several associations — that is exactly the
  shared-dependency information the current design throws away.
- **Digests everywhere.** `destination` entries are digest-pinned (see
  [pinning interplay](#pinning-interplay)).
- **`deletions` make records mutable-but-auditable.** A successful delete
  appends what was removed (and when), instead of rewriting history. Refcounts
  and repeat deletes then stay correct, and the workspace tracks the registry's
  actual state.
- **Versioning.** Records carry `apiVersion`; readers must ignore unknown
  fields so that newer writers do not break older readers within a supported
  compatibility window.

#### Lifecycle and storage

- Written at the end of `RunMirrorToDisk`, `RunMirrorToMirror`, and
  `RunDiskToMirror`, next to the existing `createConfigsWithPinnedCatalogs()`
  call sites (d2m currently writes nothing and gains this step).
- Stored under `working-dir/records/<runId>.yaml`. Since the working-dir is
  already included in the m2d archive, records ride the tarball into the
  disconnected environment for free, and d2m extracts them like the rest of the
  working-dir.
- **Destination scoping:** m2m and d2m records carry the real destination
  registry and are the ones used for refcounting. m2d records describe what went
  into the archive (their image destinations are the local cache) and primarily
  serve as provenance and audit. d2m writes *its own* record for the registry it
  pushed to (open question [below](#open-questions) on whether it should instead
  rewrite destinations on the extracted m2d records; writing its own record is
  the proposed default because refcounting by `(destination, digest)` makes the
  overlap harmless).
- **Incremental runs:** each run re-collects the full image set (today, only
  the *archive* is diffed via blob history), so records from successive runs
  overlap heavily. That is fine and intended: the union of records targeting a
  registry is exactly "everything this workspace ever mirrored there", and an
  image present in several records has a proportionally higher reference count.

#### What the record is not

The record is not a rendered `DeleteImageList` and not a replacement for the
user-facing ISC/DISC. It is internal bookkeeping with richer information
(provenance, run metadata, deletion history). The rendered, reviewable plan
remains a `DeleteImageList`-shaped document produced by the delete command (see
[the delete flow](#the-single-stage-delete-flow)); this keeps continuity with
the existing format and tooling.

### The single-stage delete flow

`oc-mirror delete` becomes one command:

```
1. LOAD      read working-dir/records/*.yaml;
             keep records whose destination is the target registry
2. SELECT    deletion candidates =
                 (run selection: --from-run <id> | --all)
               ∩ (intent filters: --config <DISC> matched against
                  record image origins/types/associations)
3. REFCOUNT  subtract images that appear in any *non-selected* record
             targeting the same registry
             → "skipped-shared" classification (escape hatch: --ignore-shared)
4. PREFLIGHT per candidate: existence check against the target registry
             → present | already-deleted ("manifest unknown") | fatal error
5. PLAN      print the classified plan and write
             working-dir/delete/delete-plan-<ts>.yaml;
             --dry-run exits here;
             otherwise prompt for confirmation on TTY (--yes when
             non-interactive is mandatory)
6. EXECUTE   delete manifests via the existing batch worker (DeleteMode);
             signature tags come from the record (no lookups needed)
7. REPORT    summary (deleted / already-deleted / shared-skipped / failed)
             + report file; append to the records' `deletions` sections
```

Step-by-step notes:

- **Select (step 2):** the DISC is demoted from *collection driver* to *intent
  filter*. Its `platform`, `operators`, `additionalImages` and `helm` sections
  are matched against the records' images (by origin reference, digest, type,
  and associations). A DISC filter that matches nothing in the records produces
  an explicit warning and a non-zero exit — never a fresh collection. A DISC
  referencing a catalog by *tag* still works: the record's pinned origins allow
  matching by either the original tag reference or the digest.
- **Refcount (step 3):** protects images that other mirror runs (other
  enclaves, other ISCs) still need. Only records in the current workspace are
  visible — see [known limitations](#known-limitations).
- **Preflight (step 4):** classification reuses the existing
  "manifest unknown" detection already present in the image-availability
  helper in `internal/pkg/mirror/mirror.go`. Transient/auth errors are fatal
  and reported as such — they must not be conflated with "already deleted"
  (issue 6).
- **Plan (step 5):** the plan file keeps the audit-trail role of today's
  generated delete yaml. It is a superset of `DeleteImageList` with a
  classification section per item (`delete` / `already-deleted` /
  `shared-skipped`).
- **Execute (step 6):** manifest deletion only, as today; remote blob cleanup
  remains the registry administrator's garbage-collection job (unchanged
  stance, documented).
- **Report (step 7):** the report is always written, even on `--dry-run`, so
  the reviewed plan and the executed report can be diffed.

**Locked-in UX decisions** (from the design review):

| Decision | Choice |
|---|---|
| Record storage (phase 1) | **Workspace only** (`working-dir/records/`), transported by the archive. Registry-side records are a follow-up option. |
| Default behavior | **Plan + confirm**: classified plan, TTY confirmation prompt; `--yes` required when non-interactive; `--dry-run` exits after the plan. |
| Missing images | **Skip + report**: preflight classifies already-deleted images, skips them with per-image reporting and a summary, exit code 0. `--strict` turns them into errors. |

Exit-code contract (proposed): `0` = deleted or cleanly skipped;
`1` = usage/configuration errors (no workspace, no records, bad DISC);
`2` = one or more deletions failed, or `--strict` missing-image failures.
Exact codes to be aligned with the existing collector exit codes during
implementation.

### Dependency safety: provenance and reference counting (issue 3)

Reference counting happens at two granularities, both derived from record data:

1. **Across runs (inter-record):** an image present in a non-selected record
   targeting the same registry is classified `shared-skipped` and protected.
   This covers "one central registry, several enclaves" *as long as all runs
   were made from this workspace* (see limitations).
2. **Within a selection (intra-record, via associations):** when the user asks
   to delete "operator package X", images whose associations include X but also
   another package/catalog/additionalImage entry are surfaced in the plan as
   shared and skipped by default. This is the "dependency not obvious from the
   DISC" case — it is only visible because the record kept the provenance.

Operator catalog interaction: when the selection deletes bundles of a package
but keeps the (rebuilt) catalog image, or deletes a catalog but not its bundles,
the plan must surface the catalog↔bundle relationship and warn that the
mirrored catalog's FBC will reference missing images (whether delete should
trigger a catalog rebuild is an [open question](#open-questions)). The record
knows both sides of this relationship (catalog digest, `rebuiltTag`,
per-bundle associations).

### Pinning interplay (issue 5)

Records are pinning taken to its conclusion:

- catalog references are pinned to digests at mirror time (existing
  OCPBUGS-81712 behavior), and every mirrored image's `destination` is recorded
  by digest — so a record is effectively a fully-pinned image set of the run;
- the pinned ISC/DISC (`isc_pinned_*.yaml` / `disc_pinned_*.yaml`) remain the
  user-facing, reproducible configuration artifacts. The record supersedes the
  ISC→DISC conversion as the delete input, because it is stricter: it is what
  was mirrored, not what was intended;
- a *pinned* DISC used as an intent filter gives exact digest-level matching
  against record origins (useful to clean up precisely one digest of a catalog
  when several were mirrored over time).

The `--delete-v1-images` flag exists precisely because v1 mirrored by tags;
records remove that ambiguity for all v2 content.

### Missing images: classification and reporting (issue 6)

| Preflight result | Meaning | Default | `--strict` |
|---|---|---|---|
| manifest present | — | delete | delete |
| `manifest unknown` | already deleted (or never mirrored to *this* registry) | skip + report, exit 0 | error |
| auth / network / other | registry or reference problem | fatal, non-zero exit | fatal |

Because successful deletions are appended to the records' `deletions`, a re-run
of the same delete is cleanly idempotent: previously deleted images classify as
`already-deleted` and are skipped without noise.

The phantom `--dry-run` referenced by today's error message becomes a real
flag of the delete command (step 5 of the flow).

### CLI surface

```bash
# intent-filtered delete over everything this workspace mirrored to the registry
oc-mirror delete --workspace file://<wd> --config disc.yaml docker://reg

# delete an entire mirror run
oc-mirror delete --workspace file://<wd> --from-run <runId> docker://reg

# delete everything this workspace ever mirrored to the registry
oc-mirror delete --workspace file://<wd> --all docker://reg

# preview / scripted
oc-mirror delete ... --dry-run
oc-mirror delete ... --yes
```

New flags: `--from-run`, `--all`, `--dry-run`, `--yes`, `--strict`,
`--ignore-shared`.

Unchanged flags: `--workspace`, `--delete-signatures`, `--force-cache-delete`,
`--delete-v1-images`.

Deprecated (still functional, see compatibility below): `--generate`,
`--delete-yaml-file`, `--delete-id`.

### Backwards compatibility and migration (issue 7)

The delete command picks its flow from the workspace state:

```
records present in workspace?
├─ yes → record-based flow (new default)
└─ no
   ├─ isc_pinned/disc_pinned present? ──> one-time "reconstruct":
   │       run the deprecated --generate (collection-based, unchanged
   │       behavior), and save its DeleteImageList *as a record*,
   │       so all subsequent deletes are record-based
   ├─ v1-mirrored content → existing --delete-v1-images + --generate path
   └─ nothing (lost ISC *and* workspace) → manual DeleteImageList authoring
           via --delete-yaml-file (already supported), or registry wipe
```

Commitments:

- The old two-stage flow keeps working for at least the deprecation window
  (timeline: open question). `--generate` and `--delete-yaml-file` are not
  removed in the first iteration.
- Records are additive: phase 1a (record writing) changes no existing behavior.
- Record readers tolerate unknown fields; the format carries `apiVersion`.
- Mirrors made by versions that predate records can be migrated with a single
  reconstruct invocation (which needs the same connectivity/caching guarantees
  as today's generate, i.e. it is no worse than the status quo).

### Known limitations

- **Refcounting only sees this workspace.** A central registry fed from several
  machines/workspaces remains unprotected against cross-workspace sharing.
  Mitigations: `--ignore-shared` escape hatch, documented recommendation to use
  one workspace per registry, and the registry-side records follow-up.
- **Catalog FBC staleness.** Deleting bundles without rebuilding the catalog
  leaves the catalog referencing missing images (warned in the plan; rebuild is
  an open question).
- **Cluster resources are not cleaned.** Deleting an image that a previously
  generated CatalogSource/IDMS/ITMS points at will break the cluster consumer;
  the plan should warn when the workspace's `cluster-resources/` reference a
  candidate (open question).
- **Local cache deletion** (`--force-cache-delete`) remains manifest-level +
  GC on the embedded registry, as today; blob-level safety in the cache is
  inherited from the registry's GC.

## Alternatives considered

- **A. Per-run `DeleteImageList` generated after m2m/d2m** — i.e. literally
  "generate the delete file at the end of the mirror run". This is the direct
  reading of the original complaint and is the conceptual precursor of this
  proposal. Rejected as-is because the `DeleteImageList` schema lacks run
  metadata and provenance, so issues 3 (shared dependencies) and 4 (lost ISC)
  would remain unsolved. The `MirrorRecord` is a superset; the rendered plan
  output remains `DeleteImageList`-shaped for continuity.
- **B. Registry-side records (push records into the mirror registry as OCI
  artifacts).** Fully solves cross-workspace sharing and lost workspaces, but
  requires a dedicated repository in every target registry, credentials, and
  user acceptance of extra content in the mirror. Deferred to a follow-up (see
  open questions).
- **C. Keep re-collection but pin everything first.** Still requires upstream
  connectivity at delete time, still cannot see what was actually mirrored
  (only what *would now* be collected), and does not help with a lost ISC.
  Rejected.

## Open questions

1. **Registry-side records (phase 2)?** If adopted: artifact format, repository
   naming convention (e.g. `<reg>/oc-mirror/records:<runId>`), auth handling,
   and whether m2d writes them at d2m time only.
2. **Catalog rebuild on delete:** should deleting bundles from a mirrored
   (rebuilt) catalog trigger a rebuild of the catalog image with the remaining
   FBC, or is a warning sufficient for phase 1?
3. **Cluster-resources awareness:** should the delete plan/warnings consult the
   workspace's generated `cluster-resources/` (CatalogSource, IDMS/ITMS) and
   flag candidates that clusters still reference?
4. **d2m record strategy:** confirm "d2m writes its own record" over
   "rewrite destinations on extracted m2d records".
5. **Naming:** `MirrorRecord` vs `ImageSetRecord` vs `MirrorSet`; directory
   name (`records/`); rendered plan file naming (`delete-plan-<ts>.yaml`).
6. **Deprecation timeline** for `--generate` / `--delete-yaml-file` /
   `--delete-id`.
7. **Exit codes:** align the proposed `0/1/2` contract with the existing
   collector exit codes (2, 4, 20, …).
8. **Cache deletion semantics** under records: should `--force-cache-delete`
   also refcount against other records' blobs (history) before GC?
9. **Helm:** confirm helm images recorded and deleted with identical semantics
   (they are expected to be; called out because the current delete docs do not
   mention helm).

## Implementation impact

| Area | Change |
|---|---|
| New `internal/pkg/records` | Record types, read/write, versioning, `deletions` append, refcount query helpers |
| `internal/pkg/cli/executor.go` | Write a record at the end of `RunMirrorToDisk`, `RunMirrorToMirror`, `RunDiskToMirror` (next to `createConfigsWithPinnedCatalogs()`; d2m gains the call) |
| `internal/pkg/delete` | New flow: select / refcount / preflight / plan / report; `DeleteRegistryImages` refactored onto records; confirm prompt |
| `internal/pkg/cli/delete.go` | New flags (`--from-run`, `--all`, `--dry-run`, `--yes`, `--strict`, `--ignore-shared`); flow selection (records vs legacy); deprecation warnings |
| `internal/pkg/mirror` | Reuse/expose the manifest-unknown classification for preflight |
| Batch worker | Largely unchanged (DeleteMode already exists); preflight classification happens before it |
| Tests | Unit: `records` package, delete flow (select/refcount/preflight/plan), CLI flag/flow selection. Integration: extend `tests/integration/delete_test.go` (record-based delete, missing-image classifications, shared-image protection, legacy fallback). E2E: record round-trip through m2d → archive → d2m → delete |

## Rollout plan

- **Phase 1a — record writing (additive, zero behavior change).** All three
  mirror workflows write records. Shippable independently; the sooner it ships,
  the sooner existing workspaces start accruing delete capability.
- **Phase 1b — record-based single-stage delete.** New flow as default when
  records exist; legacy two-stage flow intact as fallback; reconstruct path for
  pre-record workspaces; deprecation warnings (no removal).
- **Phase 2 (follow-ups, each independently valuable).** Registry-side records;
  catalog rebuild on delete; cluster-resources awareness; v1 reconstruct
  tooling beyond the existing `--delete-v1-images` flag.
