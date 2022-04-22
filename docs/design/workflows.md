# oc-mirror Workflows Overview
===
- [# oc-mirror Workflows Overview](#-oc-mirror-workflows-overview)
  - [Workflows](#workflows)
    - [Heads-Only](#heads-only)
    - [Full](#full)
    - [Ranges](#ranges)
  
## Workflows
1. Heads-Only
2. Full
3. Ranges

### Heads-Only

The heads-only work flow is the default for both catalog and platform content types. This means that
when initially publishing an imageset only the channel heads will be pulled for each content type. On the
subsequent runs, the latest versions will be mirrored.

### Full

The full mode is enabled on catalog and platform content types when the full key is set to true.
This will mirror a full catalog or release channel.

### Ranges

Version ranges can be specified when filtering catalogs by package or using release channels.
The keys are `minVersion` and `maxVersion`. The version of the platform or operator will mirror between the range with the specified
minimum and maximum versions included. 


