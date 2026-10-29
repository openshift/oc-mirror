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
| 3 | 2026-10-29 | Second adversarial review: **digest-confirmation rule replaces "tag first" deletion** (revision 2 would have destroyed re-pointed tags); record the digest actually written; refcount identity is `(repository, digest)`; registry-capability analysis and `--registry-records` policy; OCI 1.1 / referrers assessment (container-libs gap, oras-go availability, referrer cleanup on delete); worked example for updating an operator then deleting the old version; superseded-digest pruning; "use cases that could break" matrix |

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
  and the delete flow warns when registry records cannot be listed. Publication
  is **not guaranteed on every registry** — see
  [Registry capability](#registry-capability-can-record-publication-be-guaranteed)
  for the risk analysis, the artifact shape chosen for compatibility, and the
  `--registry-records` policy flag.
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
4. PREFLIGHT per candidate, concurrently: existence + *digest confirmation*
             against the target (see "Tag re-pointing", below)
             → present | manifest-unknown (already deleted) | superseded
               (tag now points elsewhere) | fatal error
             (auth/network errors are fatal, never "already deleted")
5. PLAN      print the classified plan and write
             working-dir/delete/delete-plan-<ts>.yaml (kind: DeletePlan);
             --dry-run exits here;
             otherwise prompt for confirmation on TTY (--yes required when
             non-interactive)
6. EXECUTE   delete per the digest-confirmation rule (below): by tag when the
             tag still resolves to the recorded digest, by digest only when it
             does not; via the existing batch worker (DeleteMode); then remove
             the image's attachments (signature tags from the record, plus
             referrers when the registry exposes them)
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

#### Tag re-pointing: the digest-confirmation rule

Revision 1 of this document proposed "delete the tag first, then the digest".
That is **unsafe**, because destination tags are reused across runs:

- a catalog referenced by tag keeps its tag at the destination — pinning sets
  `TargetTag` to the original tag (`pinSingleCatalogDigest`), so run 2 pushes
  the updated catalog to the *same* destination tag as run 1;
- the Cincinnati graph image is pushed to a fixed `:latest` tag;
- additional images with mutable tags (`:latest`, `:stable`) are re-pushed to
  the same destination tag on every run.

Deleting run 1 by tag would therefore delete the tag that now resolves to run
2's content — destroying current, wanted content. The rule is:

| Recorded tag resolves to | Action |
|---|---|
| nothing (404) | tag already gone; check the recorded digest, delete it if present |
| the **recorded** digest | safe: delete by tag (removes tag **and** manifest) |
| a **different** digest | tag belongs to a newer run: **never touch the tag**; delete the recorded digest only |

The third row is also a feature: it identifies **superseded digests** — the
previous rebuilt catalog, the previous graph image, the previous `:latest` —
which are untagged leftovers of earlier runs that no DISC-driven delete could
ever express. The plan classifies them explicitly so users can reclaim that
space (see [worked example](#worked-example-updating-an-operator-then-deleting-the-old-version)).

Note that content-derived destination tags are not affected: filtered catalogs
are pushed to `sha256-<catalog-digest>` / `sha256-<filtered-digest>` tags
(`TagRebuiltCatalogByDigestOnly`, `FilteredCatalogDigest`), so each run's
rebuilt catalog lands on its own tag.

#### Recording the digest that was actually written

The record must store the digest **as written to the destination**, not the
source digest. They differ whenever the copy rewrites the manifest list — for
example when `InstancePlatforms` filtering selects a subset of instances, or
when attestation instances are filtered out (`filterAttestationInstances`).
The batch worker currently discards the copy result, so this requires
surfacing the resulting manifest digest from `Mirror.Run`/`copy.Image` into the
record journal. Recording the source digest instead would produce records that
never match the registry.

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
   **Identity for refcounting is `(repository, digest)`**, never the digest
   alone and never the registry host alone: the same digest mirrored into
   `reg/enclave-a/ubi` and `reg/enclave-b/ubi` are two independent manifests,
   and deleting one must not be blocked by the other. Keying on the digest
   alone would over-protect to the point where nothing in a multi-enclave
   registry is ever deletable.
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

### Worked example: updating an operator, then deleting the old version

This is the most common real-world delete request, and it is worth walking
through because the answer differs depending on what the *current* mirror still
needs. Setup: catalog `C` (referenced by tag `v4.17`), package `P`.

- **Run 1** mirrors `P` 1.0 → record R1: rebuilt catalog `C@dA`, bundle `P1.0`,
  related images `{r1, r2, shared}`.
- **Run 2** mirrors `P` 2.0 → record R2: rebuilt catalog `C@dB`, bundle `P2.0`,
  related images `{r3, shared}`.

**Case A — run 2's ISC selects only 2.0 (the user genuinely moved on).**
`oc-mirror delete --config disc.yaml docker://reg` with a DISC selecting
package `P`, `maxVersion: 1.0`:

- SELECT matches `P1.0` and the images associated with that bundle version
  (`associations[].bundleVersion`), i.e. `{P1.0, r1, r2, shared}`;
- REFCOUNT removes `shared` (present in R2) → `shared-skipped`;
- the catalog image is **not** selected (its associations carry no
  `bundleVersion`; catalogs are only selected when the catalog itself, or the
  whole run, is selected);
- PREFLIGHT finds `C@dA` superseded (the `v4.17` tag now resolves to `dB`) and
  offers it as an orphaned digest;
- result: `P1.0`, `r1`, `r2` deleted; `shared` kept; the current catalog and
  2.0 content untouched. **This works, and it is exactly what the old design
  could not do reliably** — a DISC-driven collection would have re-resolved
  `C:v4.17` to the *new* catalog and never found 1.0's images.

**Case B — run 2's ISC still includes 1.0 (range 1.0–2.0).**
Everything in `{P1.0, r1, r2}` is also in R2, so the whole selection is
classified `shared-skipped` and **nothing is deleted**. That is correct: the
current rebuilt catalog's FBC still references 1.0, and deleting it would break
installs/upgrades for those versions. But it is also the single most likely
source of "delete is broken" bug reports, so the plan must say so explicitly,
naming the record that still needs the images, e.g.:

```
skipped (still referenced): registry/.../p-operator@sha256:…
  required by run 7f3c… (m2m, 2026-11-02) — narrow the version range in your
  ImageSetConfiguration and re-mirror, or re-run with --ignore-shared
```

**Case C — filtered vs. unfiltered catalogs.** The record never has to parse
the rebuilt FBC to know whether 1.0 is still needed: "still needed" is simply
"present in a non-selected record". Filtering therefore needs no special
handling, and a catalog that was *not* filtered behaves identically.

**Case D — two enclaves disagree.** Enclave A (workspace A) still mirrors 1.0;
enclave B wants it gone. Because run A published its record to the registry,
B's delete sees it and protects 1.0. Without registry-published records this
case silently corrupts enclave A — which is why publication is in phase 1.

**Orphaned/superseded content.** Re-mirroring a tag-referenced catalog leaves
the previous rebuilt catalog manifest untagged in the registry; the same is
true for the graph image and any mutable-tag additional image. These
accumulate with every run and are invisible to a DISC-driven delete. Records
make them addressable: the plan can classify "recorded digest no longer
referenced by its tag, superseded by a newer record" and offer them for
pruning. Whether this should be opt-in (`--prune-superseded`) or part of the
default plan is an open question.

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

### Registry capability: can record publication be guaranteed?

**No — and the design must not pretend otherwise.** Publishing an artifact to
an arbitrary disconnected registry is best-effort, for several independent
reasons:

| Risk | Effect |
|---|---|
| **Artifact-type gating** — some registries (notably Red Hat Quay, historically via `FEATURE_GENERAL_OCI_SUPPORT` / `ALLOWED_OCI_ARTIFACT_TYPES`) only accept an allow-list of OCI artifact types | push rejected |
| **No repository auto-creation** — e.g. AWS ECR requires the repository to exist before push; some registries require explicit namespace creation or an admin-owned namespace | push rejected |
| **Credentials scoped to mirrored repositories** — the mirroring robot account may have no write access outside the content namespaces | push rejected |
| **Tag listing restricted/absent** — record discovery relies on listing tags of the records repository | records invisible at delete time |
| **Tag immutability / retention policies** (Harbor, Artifactory, ECR) | per-run tags are unique so pushes are fine, but any mutable index tag or deletion-marker update fails |
| **DELETE disabled** (`storage.delete.enabled=false` on distribution, RBAC elsewhere) | the delete itself fails — a pre-existing constraint, unchanged by this design |
| **Read-only mirrors / pull-through caches** | publication impossible |
| **Scanners, quotas, policy engines** | an unexpected `oc-mirror/records` repository may be flagged, quota-charged, or garbage-collected |

Consequences for the design:

1. **Maximum-compatibility artifact shape.** The record is pushed as a plain
   **OCI image manifest** with a tiny config blob and a single layer holding
   the record document — the shape cosign/ORAS use for broad compatibility —
   *not* the deprecated OCI 1.1 `artifact.manifest` media type, which older
   registries reject. `artifactType` is set when supported but is never
   required for correctness.
2. **Capability detection, not assumption.** The mirror run probes
   publication once and records the outcome (`published: true|false`); the
   delete command probes list/read access and reports the result in the plan
   header. The plan states, in plain words, whether cross-workspace protection
   is in effect:
   `registry records: unavailable (tag listing denied) — shared-image protection is limited to this workspace`.
3. **Explicit policy flags.** `--registry-records=auto|off|require`
   (default `auto`): `off` for registries/policies that forbid it,
   `require` for shared central registries where silently losing protection is
   unacceptable (the mirror run fails if publication fails). The records
   repository path is configurable.
4. **A conformance matrix is required before GA**, validated empirically
   rather than assumed: distribution 2.8/3.x, Red Hat Quay and
   `mirror-registry`, quay.io, Harbor, Artifactory, Nexus, ECR, ACR, GAR, plus
   a "DELETE disabled" and a "tag listing denied" configuration. Each entry
   must record: artifact push, tag listing, per-tag pull, manifest delete by
   tag, manifest delete by digest, referrers API support.
   *(This matrix could not be filled in from this sandbox — no outbound
   network access — so it is explicitly listed as pre-implementation work.)*

Because publication can fail, registry records are an **enhancement of**, not
a replacement for, workspace records: the workspace remains the primary store,
and the delete flow degrades to workspace-only protection with a loud warning.

### What OCI 1.1 could give us

**Current state of the dependency.** `go.podman.io/image/v5` (container-libs)
does **not** implement OCI 1.1 referrers today: there is no referrers client in
the docker transport, and oc-mirror already carries a workaround for OCI 1.1
attestation manifests (`filterAttestationInstances` in
`internal/pkg/mirror/mirror_attestation.go`, with a `TODO ... when OCI1.1 is
supported by containers-libs [OCPNODE-2018]` in `mirror.go`). So nothing in the
current transport stack can be relied on for referrers.

**But a client is already in the dependency graph.** `oras.land/oras-go/v2`
(v2.6.2) is present as an indirect dependency (pulled in by Helm's registry
client) and implements `Repository.Referrers()` **with automatic fallback to
the referrers tag schema** when a registry does not support the Referrers API.
Promoting it to a direct dependency is therefore a small step, and it is the
pragmatic path for both record publication and referrer handling without
waiting for OCPNODE-2018.

Two distinct opportunities, with very different cost/benefit:

1. **Referrer-based provenance (optional, evaluate later).** Instead of (or in
   addition to) a per-run record artifact, attach a small provenance referrer
   to each mirrored image (`subject` = the image manifest). Reference counting
   then becomes a registry-native query — "who refers to this image?" — which
   is exact across workspaces and needs no tag listing.
   *Cost:* one extra manifest + blob per image per run; for a 10k-image
   operator mirror that is a very large request amplification, and on
   registries without the Referrers API the fallback writes a tag per subject
   (tag pollution, update races, breakage under tag immutability). A sensible
   middle ground is to attach referrers only to *roots* (catalogs, release
   payloads, additional images) and keep per-image provenance in the per-run
   record.
2. **Referrer cleanup on delete (recommended regardless of records).** Deleting
   a manifest does **not** delete artifacts that reference it. oc-mirror today
   only knows the cosign *tag* scheme (`SigstoreAttachmentTag`), so any
   referrer-attached signature, SBOM or attestation in the destination registry
   is orphaned by a delete. The delete flow should enumerate referrers of each
   deleted manifest (API when available, fallback tag otherwise) and remove
   them as part of the same operation. This is a correctness fix that stands on
   its own merits.

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
(alias `--fail-on-missing`), `--ignore-shared`, `--plan-file`,
`--prune-superseded` (include orphaned digests whose tag has moved on).

New mirror-side flag: `--registry-records=auto|off|require` (also meaningful
for `delete`, where it controls whether missing registry records are a warning
or an error).

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

### Use cases that could break under this design

These are the failure modes reviewers should attack; each needs a test.

| # | Use case | Risk | Mitigation in this design |
|---|---|---|---|
| 1 | Catalog/graph/additional image re-mirrored to the **same destination tag** | deleting an old run by tag destroys current content | digest-confirmation rule (tag deleted only when it still resolves to the recorded digest) |
| 2 | **Platform-filtered** or attestation-filtered copies | destination manifest digest ≠ source digest; records would never match the registry | record the digest actually written, surfaced from the copy result |
| 3 | Same digest mirrored into **several repositories/enclaves** | refcount keyed on digest alone makes everything undeletable | refcount identity is `(repository, digest)` |
| 4 | `--max-nested-paths` rewriting destinations | records could store pre-rewrite references | records are written from the post-rewrite `copiedSchema` |
| 5 | Registry that **rejects artifact push** (ECR without pre-created repo, gated Quay, read-only mirror) | cross-workspace protection silently absent | capability probe, `published: false`, plan header states the degradation, `--registry-records=require` for strict environments |
| 6 | Registry with **tag listing denied** | records exist but cannot be discovered | explicit warning; workspace records still used; conformance matrix entry |
| 7 | **Mixed-version content** (mirrored before records existed) | unprotected and unselectable | legacy flow retained; reconstruct path; the plan flags unrecorded repositories |
| 8 | Archives applied from **separate directories/invocations** | disjoint record sets | documented invariant; d2m imports records from every archive it processes |
| 9 | Users deleting via the **registry UI** or an external GC | records stale | staleness only over-protects; preflight reconciles actual existence |
| 10 | **Signatures/SBOMs attached as referrers** by other tooling | orphaned attachments after delete | enumerate and delete referrers (see OCI 1.1 section) |
| 11 | Operator version still referenced by the **current** catalog (Case B) | user expects deletion, gets a no-op | plan explains which record still requires each image and what to do |
| 12 | Concurrent mirrors to one registry from different workspaces | record publication races | per-run immutable tags; no mutable index tag in phase 1 |
| 13 | Registries with **DELETE disabled** | delete fails (pre-existing) | unchanged; detected in preflight as a fatal, clearly-reported error |
| 14 | Quay time-machine / deferred GC | space not reclaimed immediately; tags recoverable | documentation; out of oc-mirror's control |
| 15 | Deleting content that was **promoted/copied** to another repository by the user | the promoted copy survives (separate manifest), shared blobs survive GC | no action needed; documented |

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
1. **Referrers adoption level:** keep per-run record artifacts only (proposed),
   or additionally attach per-image provenance referrers for registry-native
   refcounting? If the latter, roots-only or all images, and behind which flag?
1. **Promote `oras.land/oras-go/v2` to a direct dependency** for artifact
   push/pull and referrer handling, or wait for container-libs OCI 1.1
   (OCPNODE-2018) and ship records workspace-only until then?
1. **Referrer cleanup on delete:** adopt in phase 1b (recommended) or defer?
1. **Superseded digests:** opt-in `--prune-superseded`, or part of the default
   plan with explicit classification?
1. **Default for `--registry-records`:** `auto` (degrade with a warning) or
   `require` when the destination is known to host several enclaves?
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
| `internal/pkg/mirror` | Expose `Mirror.Check` through the interface used by the delete preflight; surface the **digest actually written** by `Mirror.Run` so records match the registry when manifest lists are rewritten (platform/attestation filtering) |
| Registry artifact client | Record push/list/pull and referrer enumeration via `oras.land/oras-go/v2` (already an indirect dependency through Helm; container-libs has no OCI 1.1 referrers support — OCPNODE-2018), using the compatibility-first image-manifest artifact shape |
| Batch worker | Largely unchanged (DeleteMode already exists); hook to append journal entries (including written digests) as copies succeed |
| Association assembly | Join `collectorSchema.CopyImageSchemaMap` with `copiedSchema` (the worker does not return the maps today) and merge associations for images collapsed by `removeDuplicatedImages` |
| Tests | Unit: records (journal, publish, validation), delete flow (select/refcount/preflight/plan/plan-file), CLI flow selection. Integration: extend `tests/integration/delete_test.go` (record-based delete, missing-image classifications, shared-image protection across two workspaces sharing a registry, legacy fallback, plan/DeleteImageList rejection). E2E: record round-trip m2m → registry; m2d → archive → d2m → registry → delete; lost-workspace delete from registry records |

## Rollout plan

- **Phase 0 — registry conformance matrix.** Validate artifact push, tag
  listing, per-tag pull, delete by tag, delete by digest and referrers support
  across the target registries before committing to the publication design.
  This gates the phase-1a scope (it may demote registry records to `auto`-only
  on some platforms).
- **Phase 1a — record writing and publication (additive, existing behavior
  unchanged).** m2m and d2m journal and finalize records into the workspace and
  publish them to the registry (capability-probed, `--registry-records`).
  Requires surfacing written digests from the copy path. Shippable
  independently; the sooner it ships, the sooner registries start accruing
  delete capability.
- **Phase 1b — record-based single-stage delete.** New flow as opt-in via the
  new flags (`--from-run`, `--all`, `--config` without `--generate`,
  `--plan-file`); digest-confirmation deletion; referrer cleanup of deleted
  manifests; legacy two-stage flow untouched; reconstruct path for pre-record
  workspaces; deprecation warnings (no removal).
- **Phase 2 (follow-ups, each independently valuable).** Catalog rebuild on
  delete; cluster-resources awareness; v1 reconstruct tooling beyond the
  existing `--delete-v1-images` flag; record compaction/retention automation.
