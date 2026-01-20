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

### `/pr-tests`

Intelligent testing recommendations for your Pull Requests, powered by Claude Code.

An advanced Claude Code skill that analyzes GitHub Pull Requests and generates comprehensive, context-aware testing strategies tailored to your code changes.

### Why Use This?

- **QE-Focused Analysis** - Tailored for Quality Engineering.
- **No Code Output** - Shows only line numbers and high-level descriptions, no code snippets
- **Comprehensive Test Strategy** - Integration, E2E, regression, manual, and performance testing recommendations
- **Risk Assessment** - Identifies high-risk areas requiring critical testing
- **Impact Analysis** - Understands scope, dependencies, and user workflows affected
- **Multi-Language** - Supports Go, Python, JavaScript/TypeScript, Java, Ruby, Rust, and more


## Key Features

| Feature | Description |
|---------|-------------|
| **Zero Configuration** | Works out of the box as an instruction-based skill |
| **QE Test Strategy** | Focuses on integration, E2E, regression, and manual testing |
| **No Code Output** | Test approach descriptions only, no test implementations |
| **Comprehensive Analysis** | PR overview, change impact, risk assessment, and scope detection |
| **Smart Prioritization** | Identifies P0/P1/P2 test scenarios based on risk |
| **Framework Detection** | Automatically identifies language and testing frameworks |
| **Multi-Language Support** | Handles Go, Python, JS/TS, Java, Ruby, Rust and more |
| **Edge Case Discovery** | Suggests edge cases like concurrent access, permission issues, error conditions |
| **Test Environment Guidance** | Setup requirements, test data needs, and configuration |

## Installation

### Prerequisites


1. Authenticate GitHub CLI:
```bash
gh auth login
```

## Usage

### Basic Usage

1. Navigate to your project directory:
```bash
cd /path/to/your/project
```

2. Start Claude Code:
```bash
claude-code
```

3. Run the analysis:
```bash
/pr-tests https://github.com/owner/repo/pull/123
```

### Command Format

```
/pr-tests <PR_URL>
```

Where `<PR_URL>` is the full GitHub pull request URL.

## Example Output

### Input

```bash
/pr-tests https://github.com/openshift/oc-mirror/pull/1289
```

### Output

```markdown
● 🧪 QE PR Testing Strategy Analysis

  PR Overview

  - PR Number: #1289
  - Title: OCPBUGS-56398: Removes the clean up of logs directory
  - Author: Alex Guidi (@aguidirh)
  - Branch: ocpbugs-56398
  - Change Type: Bug fix
  - Language: Go
  - File Classification: ✅ Source file (needs testing)

  Changed Lines Summary

  v2/internal/pkg/cli/executor.go:
  - v2/internal/pkg/cli/executor.go:1063-1064 (removed)

  The change removes 2 lines that performed automatic cleanup of the logs directory.

  Change Analysis

  v2/internal/pkg/cli/executor.go

  What Changed:
  - Removed automatic deletion of the logs directory during initialization
  - The setupLogsLevelAndDir() function no longer calls os.RemoveAll(o.LogsDir)
  - Logs directory is now preserved between executions

  Impact and Scope:
  - CRITICAL behavioral change: Log persistence across multiple oc-mirror operations
  - Affects all oc-mirror commands: mirror, delete, list, and any future commands
  - Changes system behavior from "clean slate each run" to "accumulate logs over time"
  - Impacts troubleshooting workflows (positive - historical logs retained)
  - Impacts disk space management (logs accumulate until manually cleaned)
  - May affect automated workflows expecting fresh logs directory

  Dependencies and Integrations Affected:
  - Log aggregation or monitoring systems expecting specific log patterns
  - Automation scripts that parse logs or expect clean log directories
  - Disk space management and cleanup procedures
  - Backup/restore operations involving working directory
  - Any tooling that relies on log file naming conventions

  QE Testing Recommendations

  Test Scenarios by Changed Files

  Component: Log Management in oc-mirror Executor

  Suggested Test Scenarios (QE Perspective):

  1. Verify logs persistence across multiple mirror operations
    - Run multiple mirror operations sequentially
    - Verify logs from each operation are preserved and accessible
    - Confirm logs don't overwrite or conflict with each other
  2. Test logs accumulation over extended usage
    - Perform 10+ consecutive oc-mirror operations (mirror, delete, list)
    - Verify all log files are retained
    - Check log directory structure and organization
  3. Verify log directory creation on first run
    - Execute oc-mirror on fresh working directory
    - Confirm logs directory is created with correct permissions
    - Verify logs are written successfully
  4. Test behavior with pre-existing logs directory
    - Execute oc-mirror with existing logs directory containing previous logs
    - Verify existing logs are NOT deleted
    - Verify new logs are added alongside old logs
  5. Verify log file naming and uniqueness
    - Run multiple operations in quick succession
    - Confirm each operation generates unique log files
    - Verify no log file overwrites or conflicts occur
  6. Test disk space impact from log accumulation
    - Run oc-mirror operations over extended period
    - Monitor disk space usage in logs directory
    - Verify system behavior when disk approaches capacity
  7. Verify backward compatibility with existing workflows
    - Test integration with existing automation/CI pipelines
    - Verify log parsing tools still function correctly
    - Confirm monitoring/alerting systems handle accumulated logs
  8. Test log retention in error scenarios
    - Trigger failures during oc-mirror operations
    - Verify error logs are preserved for debugging
    - Confirm logs from failed operations remain accessible
  9. Verify multi-user/concurrent execution handling
    - Run multiple oc-mirror instances with same working directory
    - Verify log isolation or proper handling of concurrent writes
    - Check for race conditions or corruption
  10. Test log directory permissions and access
    - Verify logs directory created with correct permissions (0755)
    - Test read access to accumulated logs
    - Verify behavior with restricted permissions

  Edge Cases to Cover:
  - Logs directory exists as a file instead of directory
  - Logs directory is a symbolic link
  - Working directory doesn't exist or is inaccessible
  - Insufficient disk space for log accumulation
  - Logs directory with thousands of existing files
  - Concurrent writes to logs directory from multiple instances
  - Logs directory with restricted permissions (read-only, no write)
  - Very long running operations generating large log files
  - System restart or crash during logging operation

  Error Conditions to Handle:
  - Permission denied when accessing logs directory
  - Disk full during log write operation
  - Invalid working directory path
  - Corrupted existing logs directory
  - Network filesystem latency/failures (if working dir on network storage)

  Integration Points to Verify:
  - Integration with oc-mirror mirror command
  - Integration with oc-mirror delete command
  - Integration with oc-mirror list command
  - Integration with log level configuration (--log-level flag)
  - Integration with working directory configuration
  - Integration with CI/CD pipelines
  - Integration with log monitoring tools
  - Integration with backup/restore procedures

  User Workflows Affected:
  - Debugging failures by reviewing historical logs
  - Monitoring oc-mirror operations over time
  - Troubleshooting recurring issues using accumulated logs
  - Disk space management and cleanup procedures
  - Automated log collection for support cases
  - Log rotation and archival processes

  Test Strategy

  Test Levels:
  - Regression Testing (CRITICAL): Verify the bug fix - logs are NOT deleted
  - Integration Testing (HIGH): Test with all oc-mirror commands (mirror, delete, list)
  - E2E Testing (HIGH): Full user workflows from installation through multiple operations
  - Manual Testing (MEDIUM): Exploratory testing of edge cases and error scenarios
  - Performance Testing (MEDIUM): Log accumulation impact over extended usage

  Coverage Goals:
  - 100% coverage of all oc-mirror commands with the changed behavior
  - Verify all supported platforms (Linux, macOS, Windows if applicable)
  - Test both fresh installations and upgrades from previous versions
  - Cover all error scenarios related to log directory operations

  Priority Areas:
  1. P0 - CRITICAL: Regression test - verify logs are preserved, not deleted
  2. P0 - CRITICAL: Verify backward compatibility with existing workflows
  3. P1 - HIGH: Multi-operation scenarios (3+ consecutive runs)
  4. P1 - HIGH: Integration with all oc-mirror commands
  5. P2 - MEDIUM: Disk space management and large-scale accumulation
  6. P2 - MEDIUM: Concurrent execution scenarios
  7. P3 - LOW: Edge cases with permissions and filesystem issues

  Risk Assessment:
  - HIGH RISK: Breaking existing automation expecting clean logs
  - HIGH RISK: Disk space exhaustion from uncontrolled log growth
  - MEDIUM RISK: Log file conflicts or overwrites in concurrent scenarios
  - MEDIUM RISK: Performance degradation with thousands of log files
  - LOW RISK: Backward compatibility issues with older oc-mirror versions

  Test Scenario Descriptions

  Additional scenarios to consider:

  - "Verify end-to-end mirror workflow preserves logs from previous runs"
  - "Test system behavior when logs directory reaches gigabytes of data"
  - "Verify error messages guide users to retained logs for troubleshooting"
  - "Test upgrade scenario from version that deleted logs to new version"
  - "Verify log retention across system restarts and crashes"
  - "Test integration with log rotation tools and practices"
  - "Verify documentation and user guidance updated for new behavior"

  QE Testing Approach Guidance

  Test Environment Considerations

  Required Test Environment Setup:
  - Multiple test environments: fresh install, upgrade from previous version
  - Different operating systems: RHEL, Fedora, Ubuntu, macOS
  - Various storage configurations: local disk, NFS, cloud storage
  - Different filesystem types: ext4, xfs, NTFS
  - Sufficient disk space for log accumulation testing

  Test Data Requirements:
  - Sample image sets for mirror operations
  - Pre-existing logs directory with various file counts (0, 10, 100, 1000 files)
  - Test working directories with different permission configurations
  - Large and small mirror operations to generate varying log sizes

  External Dependencies and Integrations:
  - GitHub CLI (gh) for PR analysis
  - Container registries for mirror operations
  - File system monitoring tools for disk space tracking
  - Log parsing/analysis tools used in production

  Configuration Needed for Testing:
  - Multiple working directory configurations
  - Different log level settings (debug, info, warning, error)
  - Various oc-mirror command configurations
  - CI/CD pipeline integration configurations

  Files Not Requiring Tests

  N/A - Only one source file was modified and it requires comprehensive QE testing due to the behavioral change in log management.

  ---
  Summary

  This is a bug fix with CRITICAL impact on operational behavior. The removal of automatic log cleanup at v2/internal/pkg/cli/executor.go:1063-1064 fundamentally changes how oc-mirror manages logs - from ephemeral (deleted each run) to persistent (accumulated over time).

  Key QE Focus Areas:
  1. Regression testing to confirm the fix works as intended
  2. Backward compatibility with existing automation and workflows
  3. Disk space management implications
  4. Multi-operation scenarios to verify log accumulation works correctly
  5. Integration testing across all oc-mirror commands

  This change requires thorough QE validation before release due to its impact on debugging workflows, disk space usage, and existing automation that may depend on the previous behavior.
```

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
