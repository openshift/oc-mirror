Developers: Working with the test indexes/bundles/operators
====
- [Developers: Working with the test indexes/bundles/operators](#developers-working-with-the-test-indexesbundlesoperators)
  - [A primer on why](#a-primer-on-why)
  - [Catalog structure](#catalog-structure)
    - [A thought exercise about the ways that indexes could be published and captured by `oc-mirror`](#a-thought-exercise-about-the-ways-that-indexes-could-be-published-and-captured-by-oc-mirror)
  - [Test catalog sources of truth](#test-catalog-sources-of-truth)
## A primer on why

Some of the most complex logic in `oc-mirror` is related to resolving operator upgrade graphs, especially in relation to differential upgrades. Although Operator Registry is vendored and the API there is leveraged for point-in-time calculations, the logic for handling snapshots with a temporal component is `oc-mirror`'s native domain. That is, if you mirror an operator from a catalog, publish it, then perform a differential mirror to collect updates a not insignificant amount of work goes into designing a new catalog in which your previous mirror and the new one are organized into a valid upgrade graph for your environment.

The complexity of completing this mapping relies on the catalog publishers to some extent, but also relies on the operator publishers who are generating the Bundles that include the CSVs, annotations, and other metadata about their operator release and describing their release channels in these bundles. Because the "in-the-wild" catalogs will index bundles from operator publishers, `oc-mirror` has and will undoubtedly continue to encounter some edge cases in upgrade graph behavior which were not considered in its design.

For this reason, we must make some effort to maintain test catalogs made of bundles under our control. If an issue is identified in a live catalog (which we cannot control temporally), that issue should be replicated in our test data to ensure that `oc-mirror` behaves well when it encounters that set of catalogs on live data.

## Catalog structure

Operator Lifecycle Manager tracks the state and upgrade status of operators using a number of Kubernetes custom resources, but when it interacts with a catalog (also sometimes called an index or a registry) API in order to respond to or generate some of those resources, it [expects](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/architecture.md#catalog-registry-design) the following organization:

```
Package {name}
  |
  +-- Channel {name} --> CSV {version} (--> CSV {version - 1} --> ...)
  |
  +-- Channel {name} --> CSV {version}
  |
  +-- Channel {name} --> CSV {version}
```

A Package would be the name of the operator (for example [jaeger-operator](https://github.com/jaegertracing/jaeger-operator)). A Channel would be a named release channel into which upgrades get published. There may be many channels or just one (for example, maybe [stable](https://github.com/jaegertracing/jaeger-operator/blob/431038e8770b28dc3b0ca586fce06845f6329eb6/bundle/metadata/annotations.yaml#L7). A ClusterServiceVersion (CSV) would be derived from a single bundle published by the operator maintainer, and that bundle (including the CSV and any annotations on the bundle) would declare a few things (which matter to this graph):
  - The channels to which it should belong
  - The previous versions that it replaces
  - Any versions that may be skipped when building an upgrade graph

### A thought exercise about the ways that indexes could be published and captured by `oc-mirror`

Let's consider a notional set of snapshots to illustrate an example of the kinds of "in-the-wild" catalogs we might want to simulate.

The operator publisher for `foo` has published bundles for channel `beta` with two CSVs arranged in the channel, clearly replacing the earlier one with the latter:

```
foo
  |
  +-- beta --> foo.v0.1.0 --> foo.v0.1.1
```

If `oc-mirror` were to run with `full: false` on the index here, it should construct an index with a single channel `beta` with a reference to a single CSV `foo.v0.1.1` and should mirror only those images. `foo.v0.1.0`, and any images that its installation requires, would not have to be mirrored because `foo.v0.1.1` replaces it.

Supposing the operator publisher released a new version of `foo`, version `foo.v0.1.2` and they wanted you to be able to upgrade from either of the above directly to it. A bug was found in `foo.v0.1.1` and they don't want you to install it under normal circumstances, so in the future someone might need to install `foo.v0.1.0` and upgrade to `foo.v0.1.2`, but they don't want to orphan people who are already using `foo.v0.1.1`. The best way to do that, aware of the graph they had already published nodes for, would be to mark the "bad" release as ["skipped"](https://github.com/operator-framework/operator-lifecycle-manager/blob/15790a8a2f07fe65a3dbf5a45a54d35e20f2cce9/doc/design/how-to-update-operators.md#skipping-updates) in the upgrade graph:

```yaml
metadata:
  name: foo.v0.1.2
spec:
  replaces: foo.v0.1.0
  skips:
  - foo.v0.1.1
```

At the same time, they now believe it to be stable, so they add an extra channel to the bundle annotations and mark it as the default. Now our upgrade graph looks something like this:

```
foo                      ____________________
  |                     /                    v
  +-- beta --> foo.v0.1.0     foo.v0.1.1 --> foo.v0.1.2
  |
  +-- stable --> foo.v0.1.2
```

In the meantime, `oc-mirror` hasn't even been run, so no `foo.v0.1.2` exists in our published imageset or metadata. Now, suppose the operator publisher realizs that `foo.v0.1.2` has problems, too, and they're best solved by a refactor. So, they dutifully publish a bundle with the following (again in both channels, to get rid of the buggy publish to stable):

```yaml
metadata:
  name: foo.v0.2.0
spec:
  replaces: foo.v0.1.0
  skips:
  - foo.v0.1.1
  - foo.v0.1.2
```

This gives us an upgrade graph that now looks a bit more like this:

```
foo                      ____________________
  |                     /                    v
  +-- beta --> foo.v0.1.0     foo.v0.1.1 --> foo.v0.2.0
  |                           foo.v0.1.2 ----^
  |        foo.v0.1.2--v
  +-- stable --> foo.v0.2.0
```

The "channel head" for both channels is now v0.2.0, but the `beta` channel has two entries from the root in it with two extra nodes for which there are directed acyclic edges also pointing to the head. Another totally valid thing may occur, though. The catalog publisher has the option of "pruning" nodes from the graph to reduce the complexity of the channels while still allowing OLM to resolve upgrades from installed CSVs using just the data and metadata in the bundles that remain. This would make the catalog a bit simpler, like this:

```
foo
  |
  +-- beta --> foo.v0.2.0
  |
  +-- stable --> foo.v0.2.0
```

But, thanks to the metadata in the `foo.v0.2.0` bundle, OLM could infer the last graph and recommend an upgrade within those channels to `foo.v0.2.0`.

If, at this point in time, you decided to use `oc-mirror` to make an update of the package indexes for publishing on your disconnected side, `oc-mirror` needs to be able to understand how to construct a catalog that would enable you to do the same - but without trying to prune any `foo.v0.1.1` images or bundles from that disconnected environment.

## Test catalog sources of truth

In the [test/operator](v1/test/operator) folder, the source data exists almost entirely in the [bundles](v1/test/operator/bundles) folder. Operators are sorted there into folders named after their package. Within those folders are the bundle folders, and bundle manifests and metadata are in subfolders with the CSVs, annotations, and dependencies (because `oc-mirror` needs to map dependencies of operators for them to be usable).
                        1
The bundles are useful independently of any catalog as they represent a specific point in time for an operator publisher. We can simulate arbitrary operator publishes by generating arbitrary bundles, with upgrade graph annotations (`replaces`, `skips`, and the more complicated `skipRanges`). So, define the bundles that represent the operator publishing actions you'd like to simulate. Note that the examples given above for the `foo` package are actually bundles currently written in the test data. When we're filling out these bundles, you should follow the lead of the `foo` operator and ensure that all container image references are marked as `REGISTRY_CATALOGNAMESPACE:<operator>-<version>`, as seen for `foo-v.0.1.0` [here](test/operator/bundles/foo/foo-bundle-v0.1.0/manifests/foo.csv.yaml#L8)

Organizing those bundles into snapshot-in-time catalogs is done via definition in [the publish script](v1/test/operator/publish_images.py). The catalogs are organized into Python dictionaries with the package name as the key and the bundle versions in a list as the value of those keys. Catalog names are then defined in the [CATALOGS global variable](v1/test/operator/publish_images.py#L36) as dictionary keys, with the dictionary of packages as the value. Channels in the catalogs are built from the bundle metadata, and bundles are organized into those channels as the catalog is built.

If you need to make a change to one of the test catalogs to simulate a different point in time across updates, or to publish new bundles with different data/metadata into one of the existing catalogs, these are the places you would update them.

Execution of the script with no options will gather the specified bundles into catalogs, generate bundle images, generate fake "operator" images that only echo their originally published name and then sleep, and render and generate the catalog images. These are all pushed to the specified registry as this process goes. `CatalogSource` resources are generated per catalog and sent to STDOUT for application to a cluster.

Environment variables or command line flags can override the registry, registry repository (or catalog namespace), container runtime, catalog selection, file-based catalog architecture, and apply a catalog to a cluster automatically. The command-line flags are best documented by the help screen for the script, visible at `publish_images.py -h`. Images should only be published to the default registry and catalog namespace from `main` to ensure that the main tests are drawing from known catalogs. If you want to publish alternative catalogs/bundles with local changes to test something before merging to main, the easiest way is to use either the `REGISTRY` or `CATALOG_NAMESPACE` variable to override the defaults. For example:

```sh
REGISTRY=registry.jharmison.com ./publish_images.py
```
