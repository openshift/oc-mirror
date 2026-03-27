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

### `/test-pr`

Interactive PR testing assistant that analyzes GitHub Pull Request changes and helps plan a comprehensive testing strategy.

#### What it does

This command fetches a PR's diff using `git` (no `gh` CLI required), analyzes the changes, and walks you through an interactive testing workflow. It identifies testable functions, integration points, and user workflows affected by the PR, then provides targeted guidance for the test types you select.

#### Usage

```
/test-pr <PR_URL_or_NUMBER>
```

The PR can be specified as:
- Full URL: `https://github.com/owner/repo/pull/123`
- Short format: `owner/repo#123`
- Just the number: `123` (if in the current repo)

#### How it works

1. **Fetches the PR diff** using `git fetch origin pull/<number>/head` and analyzes files modified, scope of changes, code patterns, and existing test coverage.

2. **Asks which test types you need** (multi-select):
   - Unit Tests
   - Integration Tests
   - E2E Tests
   - Regression Tests
   - Manual Tests
   - Ad-hoc / Exploratory Tests

3. **Provides detailed guidance per test type**, including:
   - **Unit Tests** - Lists testable functions with file paths and line numbers, offers test templates, run commands, and coverage checks
   - **Integration Tests** - Identifies integration points between components, suggests scenarios, and provides run commands
   - **E2E Tests** - Maps affected user workflows (e.g., M2D, D2M, M2M for oc-mirror), provides prerequisite setup and execution steps
   - **Regression Tests** - Lists affected code paths and existing test suites to run
   - **Manual Tests** - Generates step-by-step test procedures with expected results
   - **Ad-hoc/Exploratory Tests** - Suggests focus areas, boundary conditions, and error scenarios to try

4. **Loops interactively** - After covering one function or scenario, asks what to do next: test another function, move to another test type, or finish.

#### Example

```
User: /test-pr https://github.com/openshift/oc-mirror/pull/1234

Claude: I'll analyze PR #1234 for testing requirements.
        [Fetches PR diff, analyzes changes]

        This PR modifies the mirror workflow and adds a new validation function.
        Which types of tests do you need?
        [Presents multi-select options]

User: Unit and E2E tests

Claude: For unit tests, I found these functions that need testing:
        1. ValidateImageSetConfig() in pkg/config/validate.go:45
        2. ProcessManifest() in pkg/mirror/manifest.go:123
        3. CalculateDigest() in pkg/image/digest.go:78

        Which function should we test first?

User: ValidateImageSetConfig

Claude: [Provides test template, run commands, coverage steps]

        What would you like to do next?
        - Test another function (ProcessManifest, CalculateDigest)
        - Move to E2E tests
        - Done

User: Move to E2E tests

Claude: For E2E tests, these workflows need testing:
        1. M2D workflow
        2. D2M workflow
        3. M2M workflow

        Which workflow should we test first?

[Process continues...]
```

#### Tips

- The command uses only `git` to fetch PR data, so no `gh` CLI authentication is needed
- It prioritizes tests based on risk and complexity of the changes
- If the PR already includes test files, it reviews them and suggests improvements
- Test templates follow existing project conventions and patterns

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
