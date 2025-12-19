# Generate ImageSet Configuration

Generate a v2alpha1 ImageSetConfiguration or DeleteImageSetConfiguration YAML file for oc-mirror.

## Instructions

You are helping the user create an oc-mirror configuration file. Ask the user questions to gather requirements, then generate a complete YAML configuration.

### Step 1: Configuration Type

Ask the user which type of configuration they want to generate:
- **ImageSetConfiguration** - for mirroring images (uses `mirror:` section)
- **DeleteImageSetConfiguration** - for deleting images (uses `delete:` section)

### Step 2: Components to Include

Ask which components they want to configure (can select multiple):
- **Platform** - OpenShift release versions
- **Operators** - Operator catalogs and packages
- **Additional Images** - Standalone container images
- **Helm** - Helm charts from repositories or local files

### Step 3: Gather Details for Each Component

**For Platform (if selected):**
- Channel name(s) (e.g., stable-4.18, stable-4.17, okd)
- For each channel:
  - Channel type (default: ocp, or specify okd)
  - Min version (optional)
  - Max version (optional)
- Architecture(s) (optional, e.g., amd64, arm64, ppc64le, s390x)
- Include graph data? (true/false, default: false)

**For Operators (if selected):**
- Catalog reference (e.g., registry.redhat.io/redhat/redhat-operator-index:v4.18)
- Mirror entire catalog? (full: true)
  - If no, ask for specific packages:
    - Package name
    - Min version (optional)
    - Specific channel(s) (optional)
- Repeat for additional catalogs

**For Additional Images (if selected):**
- List of image references (with tags or digests)
  - Example: registry.redhat.io/ubi8/ubi:latest
  - Example: quay.io/myorg/myimage@sha256:abc123...

**For Helm (if selected):**
- Remote repositories:
  - Repository name
  - Repository URL
  - Charts to mirror (name and version)
- Local charts:
  - Chart name
  - Local file path

### Step 4: Generate Configuration

Create a YAML file with this structure:

```yaml
---
apiVersion: mirror.openshift.io/v2alpha1
kind: <ImageSetConfiguration|DeleteImageSetConfiguration>
<mirror|delete>:
  platform:
    architectures:
      - "amd64"
    channels:
      - name: stable-4.18
        minVersion: 4.18.0
        maxVersion: 4.18.5
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: aws-load-balancer-operator
          channels:
            - name: stable-v1
  additionalImages:
    - name: registry.redhat.io/ubi8/ubi:latest
  helm:
    repositories:
      - name: podinfo
        url: https://stefanprodan.github.io/podinfo
        charts:
          - name: podinfo
            version: 5.0.0
    local:
      - name: mychart
        path: /path/to/chart.tgz
```

### Step 5: Write and Guide

After generating the configuration:
1. Write it to a file in the `isc/` folder (default: `isc/imageset-config.yaml` or `isc/delete-imageset-config.yaml`)
2. Explain how to use it with oc-mirror
3. Provide example commands based on the configuration type:

**For ImageSetConfiguration:**
```bash
# Mirror to Disk
./bin/oc-mirror -c isc/imageset-config.yaml file:///path/to/output --v2

# Disk to Mirror
./bin/oc-mirror -c isc/imageset-config.yaml --from file:///path/to/output docker://registry.example.com --v2

# Mirror to Mirror
./bin/oc-mirror -c isc/imageset-config.yaml --workspace file:///path/to/workspace docker://registry.example.com --v2
```

**For DeleteImageSetConfiguration:**
```bash
# Phase 1: Generate delete manifest
./bin/oc-mirror delete -c isc/delete-imageset-config.yaml --generate --workspace file:///path/to/workspace --delete-id my-delete docker://registry.example.com --v2

# Phase 2: Execute deletion
./bin/oc-mirror delete --delete-yaml-file /path/to/workspace/working-dir/delete/delete-images-my-delete.yaml docker://registry.example.com --v2
```

## Important Notes

- Always use `apiVersion: mirror.openshift.io/v2alpha1` (v2 only)
- ImageSetConfiguration uses the `mirror:` top-level key
- DeleteImageSetConfiguration uses the `delete:` top-level key
- All fields are optional except the ones the user explicitly requests
- Provide helpful comments in the generated YAML
- Refer to oc-mirror examples in `docs/image-set-examples/` if needed
