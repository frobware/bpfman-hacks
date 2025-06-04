# CI Automation Scripts

This directory contains automated scripts for managing GitHub pull requests using the [`autoprat`](https://github.com/frobware/autoprat) tool.

## Scripts

### `prow-ok2test-lgtm-approve`

Idempotently adds LGTM, approve, and ok-to-test labels to pull requests in the supplied repositories. This script performs a single-shot execution and is designed to be run periodically via systemd timers.

**Default repositories when run via systemd**: `openshift/bpfman-operator` and `openshift/bpfman`

**Usage:**
```bash
./prow-ok2test-lgtm-approve [-n|--dry-run] [-a|--author AUTHOR] REPO [REPO ...]
```

### `prow-override-test-fmt`

Overrides failing `test-fmt` tests by issuing override commands to Prow. This script handles repositories where formatting checks are failing and need to be bypassed. Performs a single-shot execution.

**Default repository when run via systemd**: `openshift/bpfman-operator`

**Usage:**
```bash
./prow-override-test-fmt [-n|--dry-run] [-a|--author AUTHOR] REPO [REPO ...]
```

## Common Options

All scripts support the following options:

- `-n, --dry-run`: Show what commands would be executed without actually running them
- `-a, --author AUTHOR`: GitHub author to filter PRs by (default: red-hat-konflux)

## Examples

```bash
# Dry run to see what would be executed
./prow-ok2test-lgtm-approve -n openshift/bpfman-operator

# Run with custom author
./prow-override-test-fmt -a my-author openshift/repo1 openshift/repo2

# Standard usage
./prow-ok2test-lgtm-approve openshift/bpfman-operator
```

## Systemd Integration

The scripts are designed to run as systemd user services with timers for periodic execution:

- **Services**: Execute the scripts once when triggered
- **Timers**: Schedule execution every 5 minutes
- **setup-systemd**: Management script for installing/removing systemd units

```bash
# Install and start systemd timers
./setup-systemd install

# Check status of timers and services
./setup-systemd status

# Remove timers and services
./setup-systemd remove
```

### Monitoring

```bash
# View logs for a specific service
journalctl --user -u prow-ok2test-lgtm-approve -f

# List all active timers
systemctl --user list-timers

# Check status of a specific timer
systemctl --user status prow-ok2test-lgtm-approve.timer
```

## Implementation

Both scripts use the shared `common.sh` library which provides:
- Command line argument parsing
- Single-shot execution logic  
- The `autoprat` function wrapper for consistent dry-run behaviour and logging
- Automatic injection of `-a author -r repo` arguments

## Files

- **`common.sh`**: Shared library with common functionality
- **`prow-ok2test-lgtm-approve`**: Script for adding LGTM/approve/ok-to-test labels
- **`prow-override-test-fmt`**: Script for overriding test-fmt failures
- **`setup-systemd`**: Management script for systemd units
- **`*.service`**: Systemd service unit files (oneshot execution)
- **`*.timer`**: Systemd timer unit files (5-minute intervals)