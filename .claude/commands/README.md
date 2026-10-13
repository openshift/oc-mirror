# Claude Commands for oc-mirror

This directory contains custom Claude slash commands to help with oc-mirror development and configuration.

## Available Commands

### `/generate-imageset`

Generate a v2alpha1 ImageSetConfiguration or DeleteImageSetConfiguration YAML file for oc-mirror.

#### What it does

This command provides an interactive workflow to create properly formatted configuration files for oc-mirror v2. It supports generating both mirroring configurations and deletion configurations with all available options.

#### Usage

Simply type `/generate-imageset` in your conversation with Claude Code and follow the prompts.

Claude will ask you questions about:

1. **Configuration type** - Choose between:
   - `ImageSetConfiguration` - for mirroring container images
   - `DeleteImageSetConfiguration` - for deleting mirrored images

2. **Components to include** - Select from:
   - **Platform** - OpenShift release versions and channels
   - **Operators** - Red Hat or community operator catalogs
   - **Additional Images** - Standalone container images
   - **Helm** - Helm charts from repositories or local files

3. **Specific details** - Depending on your selections:
   - Channel names and version ranges
   - Operator catalogs and package filters
   - Image references with tags or digests
   - Helm repository URLs and chart versions

#### Examples

**Example 1: Creating a simple platform mirror configuration**

```
User: /generate-imageset
Claude: Which type of configuration do you want to generate?
User: ImageSetConfiguration
Claude: Which components do you want to configure?
User: Just platform
Claude: What channel name? (e.g., stable-4.18)
User: stable-4.18
Claude: Min version?
User: 4.18.0
Claude: Max version?
User: 4.18.5
...
```

This generates:
```yaml
---
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    channels:
      - name: stable-4.18
        minVersion: 4.18.0
        maxVersion: 4.18.5
```

**Example 2: Creating a comprehensive configuration**

```
User: /generate-imageset
...
User: Platform, Operators, and Additional Images
...
```

This generates a complete configuration with multiple sections.

**Example 3: Creating a delete configuration**

```
User: /generate-imageset
Claude: Which type of configuration do you want to generate?
User: DeleteImageSetConfiguration
...
```

This generates a configuration for deleting previously mirrored content.

#### What gets generated

The command will:

1. **Create the YAML file** with proper v2alpha1 structure
2. **Write it to disk** (default names: `imageset-config.yaml` or `delete-imageset-config.yaml`)
3. **Provide usage instructions** with example commands for:
   - Mirror to Disk workflow
   - Disk to Mirror workflow
   - Mirror to Mirror workflow
   - Delete workflows (Phase 1 and Phase 2)

#### Common Workflows

**Mirroring OpenShift releases to a disconnected registry:**
```bash
# Step 1: Generate config
/generate-imageset
# Select: ImageSetConfiguration, Platform
# Specify channel and versions

# Step 2: Use the generated config
./bin/oc-mirror -c imageset-config.yaml --workspace file:///path/to/workspace docker://your-registry.com --v2
```

**Deleting old operator versions:**
```bash
# Step 1: Generate delete config
/generate-imageset
# Select: DeleteImageSetConfiguration, Operators
# Specify catalogs and versions to delete

# Step 2: Generate delete manifest
./bin/oc-mirror delete -c delete-imageset-config.yaml --generate --workspace file:///path/to/workspace --delete-id cleanup-old docker://your-registry.com --v2

# Step 3: Execute deletion
./bin/oc-mirror delete --delete-yaml-file /path/to/workspace/working-dir/delete/delete-images-cleanup-old.yaml docker://your-registry.com --v2
```

#### Tips

- You can include multiple components in a single configuration
- All fields are optional - only specify what you need
- The command generates v2alpha1 format only (oc-mirror v2)
- Refer to `docs/image-set-examples/` for more configuration examples
- For operators, you can specify entire catalogs (`full: true`) or individual packages
- Use digests instead of tags for additional images when you need reproducible mirrors

## Adding New Commands

To add a new slash command:

1. Create a new `.md` file in this directory (e.g., `my-command.md`)
2. Write the command instructions following the pattern in `generate-imageset.md`
3. The command name will be the filename without the `.md` extension
4. Users can invoke it with `/my-command`

## References

- [oc-mirror README](../../README.md)
- [oc-mirror Image Set Examples](../../docs/image-set-examples/)
- [Delete Functionality Documentation](../../docs/features/delete-functionality.md)
