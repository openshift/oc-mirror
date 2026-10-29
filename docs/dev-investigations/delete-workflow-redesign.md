# Delete workflow redesign (v2)

| | |
|---|---|
| **Status** | Proposal — under review (revision 2) |
| **Date** | 2026-10-29 |
| **Scope** | `oc-mirror delete` subcommand, mirror-run bookkeeping |
| **Target audience** | oc-mirror architects and developers |

This document proposes a redesign of the v2 delete workflow. It is intentionally
written before any implementation work so the team can review, iterate on, and
approve the design.

## Revision history

| Revision | Date | Changes |
|---|---|---|
| 1 | 2026-10-29 | Initial proposal |
| 2 | 2026-10-29 | Adversarial review incorporated: registry-published records moved into phase 1; failure-path/incremental record writes; tag+digest reference semantics; explicit legacy-vs-record flow selection; m2d excluded from record writing; selection provenance in the schema; reviewed-plan execution; record validation/trust boundary; delta-archive invariant; auth-vs-missing preflight rule |

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
- The m2d archive also contains the ISC it was built from (`isc_<timestamp>` in
  the archive) and the whole working-dir. Losing the local ISC copy is therefore
  not fatal as long as the tarball survives; the genuinely unrecoverable case is
  a lost workspace (see issue 4).
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
   registry. Note that this is already mitigated for m2d when the tarball
   survives — the archive contains the ISC — and by the pinned ISC/DISC files
   written by recent versions. The genuinely unrecoverable case is a lost
   workspace. The design must therefore treat *the registry* as the place where
   delete-critical state must be recoverable from, not the local workspace
   alone.

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
   `--delete-v1-images`), and must not silently change the behavior of existing
   delete invocations.

## Design goals / non-goals

**Goals**

- Single-stage delete: one command, with an in-band review/confirmation step.
- The delete image list is derived from **what was actually mirrored**, never
  from a fresh collection.
- Dependency safety: images shared between runs, and between workspaces that
  feed the same registry, are detected and protected by default.
- Deleting without the original ISC **or the original workspace** must be
  possible for registries whose content was mirrored by a record-publishing
  version.
- Consistent, explicit, reported behavior for images not found in the registry.
- Existing delete invocations keep their exact behavior (explicit legacy path).
- Works air-gapped, without upstream connectivity.

**Non-goals (for the first iteration)**

- Deleting blobs from the remote registry (remains the registry
  administrator's garbage-collection job).
- Cleaning up generated cluster resources (CatalogSource, IDMS/ITMS) — see
  open questions.
- v1 pruning semantics.

## Proposed design

### The core idea

The root cause of most of the problems above is that delete *derives* its image
list by re-running collection against current source state. The fix is to make
the mirror itself persist what it actually mirrored — a **`MirrorRecord`** —
published both to the workspace and to the target registry, and to redefine
delete as **set arithmetic over records**.

A `MirrorRecord` is written at the end of every successful m2m and d2m run. It
captures:

- the exact set of images that were successfully copied (the batch worker's
  `copiedSchema`), digest-resolved at mirror time;
- both the pushed reference (tag form, when one exists) and the digest;
- the **selection provenance** of each image (which release channel/version,
  operator package/channel/bundle version, additional image or helm entry
  caused it to be mirrored);
- run metadata (run id, timestamp, mode, destination, oc-mirror version,
  ISC digest, publication status).

Everything else in the design follows from this artifact.

### The `MirrorRecord` artifact

#### Schema (sketch)

```yaml
kind: MirrorRecord
apiVersion: mirror.openshift.io/v2alpha1
metadata:
  runId: 3f2a9c1e-...            # UUID
  timestamp: 2026-10-29T10:00:00Z
  mode: m2m                       # m2m | d2m
  destination: mirror.corp.local  # normalized registry host (refcount scope)
  ocMirrorVersion: 4.19.0
  iscDigest: sha256:...           # digest of the pinned ISC used for the run
  published: true                 # record was successfully pushed to the registry
images:
  - origin: registry.redhat.io/rh-osbs/...
    # both references are kept so DISC intent can match a tag even after pinning
    requestedReference: registry.redhat.io/redhat/redhat-operator-index:v4.13
    pinnedReference: registry.redhat.io/redhat/redhat-operator-index@sha256:...
    source: docker://...
    # pushed reference (tag form, if the push used one) and digest destination
    reference: docker://mirror.corp.local/...:v4.13
    destination: docker://mirror.corp.local/...@sha256:...
    type: OperatorBundle
    rebuiltTag: ""
    # why this image was mirrored — used to apply DISC intent offline
    associations:
      - kind: operator
        catalog: redhat-operator-index@sha256:...
        package: aws-load-balancer-operator
        channel: alpha
        bundleVersion: 0.0.1
      - kind: release
        channel: stable-4.13
        version: 4.13.3
        arch: amd64
      - kind: additionalImage
      - kind: helm
        repository: https://charts.example.com
        chart: my-chart
    signatures:                   # signature tags captured at mirror time
      - sha256-...  (sigstore attachment tags)
deletions:                        # appended when a later delete removes images
  - timestamp: 2026-11-05T09:12:00Z
    images:
      - docker://mirror.corp.local/...@sha256:...
```

Design notes:

- **Only successfully copied images are recorded.** The record is a journal:
  entries are appended as copies succeed, and the record is finalized in every
  exit path (see lifecycle below), so a batch that ends with errors still
  produces a faithful record of what landed in the registry.
- **Both tag and digest references are recorded** (`reference` and
  `destination`). The record is therefore also the definition of *what to
  delete*: the tag form when the push used one, the digest otherwise. This
  avoids the behavioral trap of deleting only by digest and leaving dangling
  tags (see the execute step).
- **`associations` carry intent provenance** (issue 3 and DISC-as-filter): they
  are the record-side equivalent of today's `CopyImageSchemaMap` plus the
  release/additional/helm selection data that the collectors otherwise throw
  away. An image can legitimately appear with several associations — that is
  exactly the shared-dependency information the current design loses.
- **`deletions` make records mutable-but-auditable in the workspace.** A
  successful delete appends what was removed (and when), instead of rewriting
  history. Registry-published records stay immutable (see below).
- **Versioning.** Records carry `apiVersion`; readers ignore unknown fields so
  newer writers do not break older readers within a supported compatibility
  window.

#### Lifecycle and storage

- **Written by m2m and d2m only.** These are the runs that populate a real
  registry. m2d does not write records: its archive already contains the ISC and
  the complete cache layout, and d2m publishes the record when the content
  reaches a registry. This also avoids unbounded archive growth: the working-dir
  is archived on every m2d run, so per-run records written by m2d would be
  re-archived on every subsequent run.
- **Journaling.** At batch start, a record journal is created; each successful
  copy appends an entry; a finalize step (deferred on every exit path) writes
  the complete record. A crash during the batch leaves a partial journal that
  subsequent runs can salvage rather than a gap in the history.
- **Workspace storage:** `working-dir/records/<runId>.yaml`, with atomic writes
  (temporary file + rename) and validation on read.
- **Registry storage (phase 1):** m2m and d2m also publish the record as an
  immutable OCI artifact in a configurable repository (default:
  `<registry>/oc-mirror/records:<runId>`). Publication is best-effort: on
  failure the run logs a prominent warning, marks the record `published: false`,
  and the delete flow warns when registry records cannot be listed. The exact
  repository name, auth model, tag scheme and listing fallback are open
  questions.
- **Why registry storage matters:** it is the only mechanism that protects
  images shared between *different workspaces* feeding the same central registry
  (issue 3), and it lets a user delete content after losing the original
  workspace (issue 4). The workspace copy remains the fast path and the place
  where deletion history is kept.
- **Destination scoping:** records are scoped by normalized registry host, so a
  workspace reused for several destinations does not cross-contaminate
  refcounts.
- **Reconciliation:** the delete flow's view of "what the registry contains" is
  the union of workspace records and registry records for that destination.
  Registry records being immutable means a stale record can only cause
  *over-protection* (an image already deleted elsewhere is skipped as shared),
  never under-protection. Deletions are appended to workspace records; whether
  to also publish deletion markers to the registry is an open question.
- **Incremental and delta-archive workflows:** each run re-collects the full
  image set (today, only the *archive* is diffed via blob history), so records
  from successive runs overlap heavily. That is intended: an image present in
  several records has a proportionally higher reference count. For d2m, records
  from *every archive processed in the same invocation* are imported, because
  `--from` extracts all chunk archives found in the directory into the same
  working-dir. Archives pushed separately (separate directories/invocations)
  produce disjoint record sets — the union is the best available approximation
  of registry content, not a proof of it. See
  [known limitations](#known-limitations).

#### What the record is not

The record is not a rendered delete plan and not a replacement for the
user-facing ISC/DISC. It is internal bookkeeping with richer information
(provenance, run metadata, deletion history). The rendered, reviewable artifact
is a separate `DeletePlan` document produced by the delete command (see
[the delete flow](#the-single-stage-delete-flow)); it keeps continuity with the
existing `DeleteImageList` format without being interchangeable with it.

### The single-stage delete flow

`oc-mirror delete` becomes one command:

```
1. LOAD      read workspace records and list/read registry records for the
             target registry; merge (records are immutable, identities by runId)
2. SELECT    deletion candidates =
                 (run selection: --from-run <id> | --all)
               ∩ (intent filters: --config <DISC> matched against
                  record origins and associations)
             a filter that matches nothing in the records is an explicit
             warning (never a fresh collection)
3. REFCOUNT  subtract images that appear in any *non-selected* record
             (workspace or registry) for the same destination
             → "skipped-shared" classification (escape hatch: --ignore-shared)
4. PREFLIGHT per candidate, concurrently: existence check against the target
             → present | manifest-unknown (already deleted) | fatal error
             (auth/network errors are fatal, never "already deleted")
5. PLAN      print the classified plan and write
             working-dir/delete/delete-plan-<ts>.yaml (kind: DeletePlan);
             --dry-run exits here;
             otherwise prompt for confirmation on TTY (--yes required when
             non-interactive)
6. EXECUTE   delete the recorded reference(s) for each present image:
             tag form first (removes tag + manifest), then the digest only if
             the manifest is still present afterwards (a 404 at that point is
             expected and treated as success); via the existing batch worker
             (DeleteMode); signature tags come from the record when available
7. REPORT    summary (deleted / already-deleted / shared-skipped / failed)
             + report file; append deletions to the workspace records;
             optionally publish deletion markers (open question)
```

Execution alternatives:

- `--plan-file <path>` executes a previously reviewed `DeletePlan` instead of
  recomputing selection (preserves the two-person/asynchronous review pattern
  of the old two-stage flow and avoids plan/execute TOCTOU). The plan is
  validated against the target registry and record metadata before execution;
  images that became shared since the plan was written are re-classified and
  skipped unless `--ignore-shared` was recorded in the plan.
- `--dry-run` and the plan file always use a distinct `kind: DeletePlan`.
  The legacy `--delete-yaml-file` path rejects `DeletePlan` documents; it
  continues to accept only `DeleteImageList`.

Step-by-step notes:

- **Select (step 2):** the DISC is demoted from *collection driver* to *intent
  filter*. Its `platform`, `operators`, `additionalImages` and `helm` sections
  are matched against the records' associations and references. Matching uses
  the recorded selection provenance (channel/version/arch, package/channel/
  bundle version), not fragile string matching on origin tags; both the
  requested (tag) and pinned (digest) references are available (`requestedReference`
  / `pinnedReference`), so a DISC naming a catalog by tag works even though the
  mirror used a digest.
- **Refcount (step 3):** protects images that other mirror runs (other
  enclaves, other ISCs, other workspaces) still need, using all visible records.
- **Preflight (step 4):** reuses `Mirror.Check` (`internal/pkg/mirror/mirror.go`),
  which already maps `manifest unknown` to `(false, nil)`. Checks run with the
  same concurrency as the batch worker and per-request timeouts.
- **Plan (step 5):** the plan file keeps the audit-trail role of today's
  generated delete yaml and is a superset of `DeleteImageList` with a
  classification per item (`delete` / `already-deleted` / `shared-skipped`).
- **Execute (step 6):** manifest deletion only, as today; remote blob cleanup
  remains the registry administrator's garbage-collection job (unchanged
  stance, documented).
- **Report (step 7):** the report is always written, including on `--dry-run`,
  so reviewed plans and executed reports can be diffed.

**Locked-in UX decisions** (from the design review):

| Decision | Choice |
|---|---|
| Record storage | **Workspace + registry** (phase 1). Registry records are immutable OCI artifacts; workspace records are the fast path and hold deletion history. |
| Default behavior | **Plan + confirm**: classified plan, TTY confirmation prompt; `--yes` required when non-interactive; `--dry-run` exits after the plan; `--plan-file` executes a reviewed plan. |
| Missing images | **Skip + report**: preflight classifies already-deleted images, skips them with per-image reporting and a summary, exit code 0. `--strict` (alias `--fail-on-missing`) turns them into errors. |
| Flow selection | **Explicit**: `--generate` / `--delete-yaml-file` always select the legacy path unchanged. The record-based flow is selected by `--from-run`, `--all`, `--plan-file`, or `--config` **without** `--generate` (a combination that is currently rejected by validation, so no existing invocation changes meaning). |

Exit-code contract (proposed): `0` = deleted or cleanly skipped (including a
"nothing matched" warning); `1` = usage/configuration errors (no workspace, no
records, bad DISC, refused plan-file); `2` = one or more deletions failed, or
`--strict` missing-image failures; `3` = user declined confirmation. Exact codes
to be aligned with the existing collector exit codes during implementation.

### Dependency safety: provenance and reference counting (issue 3)

Reference counting happens at two granularities, both derived from record data:

1. **Across runs and workspaces (inter-record):** an image present in a
   non-selected record for the same destination — whether the record lives in
   this workspace or in the registry — is classified `shared-skipped` and
   protected. This covers "one central registry, several enclaves" for every
   run made by a record-publishing version.
2. **Within a selection (intra-record, via associations):** when the user asks
   to delete "operator package X", images whose associations include X but also
   another package/catalog/additionalImage entry are surfaced in the plan as
   shared and skipped by default. This is the "dependency not obvious from the
   DISC" case — it is only visible because the record kept the provenance.

Residual risk: content mirrored by versions that predate records, or by runs
whose registry publication failed, is invisible to refcounting. The plan warns
when it cannot enumerate registry records and when a selection overlaps
repositories known only from the workspace, and `--ignore-shared` must be given
with the reviewed plan.

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
  OCPBUGS-81712 behavior), and every mirrored image's destination is recorded
  by digest — so a record is effectively a fully-pinned image set of the run;
- the pinned ISC/DISC (`isc_pinned_*.yaml` / `disc_pinned_*.yaml`) remain the
  user-facing, reproducible configuration artifacts. The record supersedes the
  ISC→DISC conversion as the delete input, because it is stricter: it is what
  was mirrored, not what was intended;
- the record keeps the *requested* (pre-pinning) reference alongside the pinned
  one, so a pinned DISC used as an intent filter gives exact digest-level
  matching without losing the user's original tag vocabulary.

The `--delete-v1-images` flag exists precisely because v1 mirrored by tags;
records remove that ambiguity for all v2 content.

### Missing images: classification and reporting (issue 6)

| Preflight result | Meaning | Default | `--strict` |
|---|---|---|---|
| manifest present | — | delete | delete |
| `manifest unknown` | already deleted (or never mirrored to *this* registry) | skip + report, exit 0 | error |
| auth / network / other | registry or reference problem | fatal, non-zero exit | fatal |

Because successful deletions are appended to the workspace records' `deletions`,
a re-run of the same delete is cleanly idempotent: previously deleted images
classify as `already-deleted` and are skipped without noise. Registry records
are immutable, so stale registry state can only over-protect (safe direction);
preflight resolves the actual existence before any deletion.

The phantom `--dry-run` referenced by today's error message becomes a real flag
of the delete command (step 5 of the flow).

### Record validation and trust boundary

Records are editable files in a user-controlled workspace, and can direct
deletions. The design therefore validates before acting:

- every image destination/reference must live under the record's declared
  destination registry and under the delete command's target registry (the
  existing check in `DeleteRegistryImages` is kept and extended to records);
- the record's `destination` must match the target registry;
- records are parsed strictly (known `apiVersion`, required fields), written
  atomically, and never followed blindly: the rendered plan is the human review
  gate, and `--plan-file` re-validates.

### CLI surface

```bash
# intent-filtered delete over everything this workspace/registry records know
oc-mirror delete --workspace file://<wd> --config disc.yaml docker://reg

# delete an entire mirror run
oc-mirror delete --workspace file://<wd> --from-run <runId> docker://reg

# delete everything recorded for the registry from this workspace
oc-mirror delete --workspace file://<wd> --all docker://reg

# preview / scripted / reviewed-plan execution
oc-mirror delete ... --dry-run
oc-mirror delete ... --yes
oc-mirror delete --plan-file delete-plan-<ts>.yaml docker://reg
```

New flags: `--from-run`, `--all`, `--dry-run`, `--yes`, `--strict`
(alias `--fail-on-missing`), `--ignore-shared`, `--plan-file`.

Unchanged flags: `--workspace`, `--delete-signatures`, `--force-cache-delete`,
`--delete-v1-images`.

Deprecated (still functional, see compatibility below): `--generate`,
`--delete-yaml-file`, `--delete-id`.

The record-based path does not need the embedded local registry (and its port),
except for `--force-cache-delete`; the delete command can skip starting it.

### Backwards compatibility and migration (issue 7)

Flow selection is explicit, never inferred from the presence of records:

```
--generate and/or --delete-yaml-file given?
├─ yes → legacy two-stage flow, behavior unchanged (regardless of records)
└─ no
   ├─ --from-run / --all / --config / --plan-file → record-based flow
   └─ no selection → usage error
```

`--config` **without** `--generate` is currently rejected by validation
(`the --delete-yaml-file flag is mandatory when not using the --generate flag`),
so assigning it the new meaning does not change any existing invocation.

Migration and degradation:

- **Pre-record workspaces:** run the legacy `--generate` once and save its
  `DeleteImageList` as a reconstructed record (`reconstructed: true`, no
  associations). All subsequent deletes are record-based. The plan warns that
  reconstructed records provide reduced shared-dependency protection.
- **Workspaces whose content was mirrored before the record feature:** same as
  above. If the ISC is lost but the tarball survives, the ISC can be recovered
  from the archive first.
- **Lost workspace:** if the runs published registry records, a fresh workspace
  can pull them (`oc-mirror delete` lists and reads them from the registry) and
  delete without any local state. If the runs predate records or publication
  failed, only manual `--delete-yaml-file` authoring or a registry wipe remain.
- **v1 content:** existing `--delete-v1-images` + `--generate` path, unchanged.
- **Mixed-version fleets:** newer versions write records; older versions ignore
  the `records/` directory and keep using the legacy flow. Records remain inert
  for older binaries.
- **Reference semantics in the legacy path are unchanged** (the legacy path
  keeps deleting the `DeleteImageList` references exactly as today).

### Known limitations

- **Records approximate registry content, they are not a proof.** Content from
  runs that predate records, or whose registry publication failed, is invisible
  to refcounting. Archives pushed separately (separate directories/invocations)
  produce disjoint records. The plan warns when registry records cannot be
  enumerated or when a selection touches unrecorded repositories.
- **Registry records require read access** to the records repository and a
  registry that supports artifact push, tag listing and per-tag pulls. The
  behavior when these are unavailable is an open question (proposed: warn and
  require explicit confirmation).
- **Catalog FBC staleness.** Deleting bundles without rebuilding the catalog
  leaves the catalog referencing missing images (warned in the plan; rebuild is
  an open question).
- **Cluster resources are not cleaned.** Deleting an image that a previously
  generated CatalogSource/IDMS/ITMS points at will break the cluster consumer;
  the plan should warn when the workspace's `cluster-resources/` reference a
  candidate (open question).
- **Local cache deletion** (`--force-cache-delete`) remains manifest-level + GC
  on the embedded registry, as today; blob-level safety in the cache is
  inherited from the registry's GC.

## Alternatives considered

- **A. Per-run `DeleteImageList` generated after m2m/d2m** — i.e. literally
  "generate the delete file at the end of the mirror run". This is the direct
  reading of the original complaint and is the conceptual precursor of this
  proposal. Rejected as-is because the `DeleteImageList` schema lacks run
  metadata, provenance and the tag/digest pair, so issues 3 (shared
  dependencies) and 4 (lost workspace) would remain unsolved. The
  `MirrorRecord` is a superset; the rendered plan output stays
  `DeleteImageList`-shaped for continuity (but is a distinct `DeletePlan` kind).
- **B. Registry-side records as a follow-up only.** Fully solves cross-workspace
  sharing and lost workspaces, but requires a dedicated repository, credentials
  and user acceptance of extra content in the mirror. **Adopted into phase 1**
  after review: workspace-only records do not solve the scenario that motivated
  issue 3.
- **C. Keep re-collection but pin everything first.** Still requires upstream
  connectivity at delete time, still cannot see what was actually mirrored
  (only what *would now* be collected), and does not help with a lost
  workspace. Rejected.
- **D. Use the m2d archive as the record (no new artifact).** The archive
  already contains the ISC and the cache's repository/tag layout, so a delete
  could consume tarballs directly. Rejected: it cannot reconstruct selection
  provenance or requested references, and it covers only archives the user
  still has; it is the same "which archive was applied where" problem.

## Open questions

1. **Registry record details:** repository location/naming, auth model, tag
   scheme, tag-listing fallback for registries without `/tags/list`, retention
   and compaction of per-run artifacts.
2. **Deletion markers in the registry:** keep deletion history workspace-only
   (current proposal — staleness only over-protects), or publish immutable
   deletion-marker artifacts?
3. **Behavior when registry records cannot be listed** (no read access, registry
   limitation): warn + require confirmation, or fail unless
   `--workspace-only` is passed?
4. **Catalog rebuild on delete:** should deleting bundles from a mirrored
   (rebuilt) catalog trigger a rebuild of the catalog image with the remaining
   FBC, or is a warning sufficient for phase 1?
5. **Cluster-resources awareness:** should the delete plan/warnings consult the
   workspace's generated `cluster-resources/` (CatalogSource, IDMS/ITMS) and
   flag candidates that clusters still reference?
6. **Naming:** `MirrorRecord` vs `ImageSetRecord` vs `MirrorSet`; directory
   name (`records/`); plan kind/name (`DeletePlan`); registry repository name.
7. **Deprecation timeline** for `--generate` / `--delete-yaml-file` /
   `--delete-id`.
8. **Exit codes:** align the proposed `0/1/2/3` contract with the existing
   collector exit codes (2, 4, 20, …).
9. **Cache deletion semantics** under records: should `--force-cache-delete`
   also refcount against other records' blobs (history) before GC?
10. **Helm:** confirm helm images recorded and deleted with identical semantics
    (they are expected to be; called out because the current delete docs do not
    mention helm).

## Implementation impact

| Area | Change |
|---|---|
| New `internal/pkg/records` | Record types (incl. schema versioning), journal writer, workspace storage, registry publish/list/pull, validation, refcount query helpers |
| `internal/pkg/cli/executor.go` | Journal init at batch start and finalize in a `defer` for m2m and d2m; publish records after the batch; m2m currently writes pinned configs even on batch errors while m2d returns early — the record path must be explicit and consistent |
| `internal/pkg/delete` | New flow: load/merge records (workspace + registry), select, refcount, preflight, plan (`DeletePlan`), confirm/plan-file execution, report; append deletions to workspace records |
| `internal/pkg/cli/delete.go` | New flags (`--from-run`, `--all`, `--dry-run`, `--yes`, `--strict`, `--ignore-shared`, `--plan-file`); explicit flow selection; plan-kind validation in the legacy path; deprecation warnings |
| `internal/pkg/mirror` | Expose `Mirror.Check` through the interface used by the delete preflight |
| Batch worker | Largely unchanged (DeleteMode already exists); optional hook to append journal entries as copies succeed |
| Association assembly | Join `collectorSchema.CopyImageSchemaMap` with `copiedSchema` (the worker does not return the maps today) and merge associations for images collapsed by `removeDuplicatedImages` |
| Tests | Unit: records (journal, publish, validation), delete flow (select/refcount/preflight/plan/plan-file), CLI flow selection. Integration: extend `tests/integration/delete_test.go` (record-based delete, missing-image classifications, shared-image protection across two workspaces sharing a registry, legacy fallback, plan/DeleteImageList rejection). E2E: record round-trip m2m → registry; m2d → archive → d2m → registry → delete; lost-workspace delete from registry records |

## Rollout plan

- **Phase 1a — record writing and publication (additive, existing behavior
  unchanged).** m2m and d2m journal and finalize records into the workspace and
  publish them to the registry. Shippable independently; the sooner it ships,
  the sooner registries start accruing delete capability.
- **Phase 1b — record-based single-stage delete.** New flow as opt-in via the
  new flags (`--from-run`, `--all`, `--config` without `--generate`,
  `--plan-file`); legacy two-stage flow untouched; reconstruct path for
  pre-record workspaces; deprecation warnings (no removal).
- **Phase 2 (follow-ups, each independently valuable).** Catalog rebuild on
  delete; cluster-resources awareness; v1 reconstruct tooling beyond the
  existing `--delete-v1-images` flag; record compaction/retention automation.
