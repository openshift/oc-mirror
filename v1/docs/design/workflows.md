Design: oc-mirror Workflows Overview
===
- [Design: oc-mirror Workflows Overview](#design-oc-mirror-workflows-overview)
  - [Workflows](#workflows)
    - [Heads-Only](#heads-only)
    - [Full](#full)
    - [Ranges](#ranges)
  
## Workflows
1. Heads-Only
2. Full
3. Ranges

### Heads-Only

The heads-only workflow is the default for both catalog and platform content types. 
This workflow, when initially publishing an imageset, will only mirror the channel heads for each content type. 
On the subsequent runs, bundles will be mirrored from the previous channel head to the current channel head.

Heads-only is the default workflow and is controlled by the `full` key, which is set to false by default.

### Full

The full mode is enabled on catalog and platform content types when the `full` key is set to true.
This will mirror a full catalog or release channel.

### Ranges

Version ranges can be specified when filtering catalogs by package or using release channels.
The keys are `minVersion` and `maxVersion`. The versions within the specified range and the specified
minimum and maximum versions will be included.

To be clear, the names "minVersion," "maxVersion," or latest version (HEAD) do not refer to bundle versions being sorted in a semver-style hierarchy, but rather to how versions are built in upgrade graphs for each channel through "replaces" and "skips".

If the `minVersion` or `maxVersion` keys are unset, the result is defined as follows:

1. `minVersion` is unset and `maxVersion` is set: The oldest version from the channel is set as the implicit min.
2. `maxVersion` is unset and `minVersion` is set:  The latest version from the channel is set as the implicit max.
3. `minVersion` and `maxVersion` are unset with full set to true: The full channel is mirrored.
4. `minVersion` and `maxVersion` are unset with full set to false: The latest version is set as min and max (the default).


