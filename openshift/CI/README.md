# CI Automation Scripts

This directory contains automated scripts for managing GitHub pull requests using the [`autoprat`](https://github.com/frobware/autoprat) tool.

## Systemd Automation

The primary automation is handled by systemd user services that directly call `autoprat` for each repository:

### Automated Services

- `prow-ok2test-lgtm-approve-openshift-bpfman-operator` (every 15 minutes)
- `prow-ok2test-lgtm-approve-openshift-bpfman` (every 15 minutes)  
- `prow-override-test-fmt-openshift-bpfman-operator` (every 30 minutes)
- `prow-override-test-fmt-openshift-bpfman` (every 30 minutes)

Each service runs with `--author red-hat-konflux` and targets its specific repository.

## Manual Scripts

The following scripts provide convenience wrappers for manual command-line usage:

### `prow-ok2test-lgtm-approve`

Convenience script that shows what commands would be executed for LGTM, approve, and ok-to-test actions. Pipe to `sh` to execute them.

**Usage:**
```bash
./prow-ok2test-lgtm-approve [--author AUTHOR] --repo REPO
```

### `prow-override-test-fmt`

Convenience script that shows what commands would be executed for test-fmt overrides. Pipe to `sh` to execute them.

**Usage:**
```bash
./prow-override-test-fmt [--author AUTHOR] --repo REPO
```

**Note:** If `--author` is not specified, both scripts automatically use your GitHub username (via `gh api user --jq .login`).

## Examples

```bash
# Show what commands would be executed (uses your GitHub username automatically)
./prow-ok2test-lgtm-approve --repo openshift/bpfman-operator

# Execute the commands if satisfied with the output
./prow-ok2test-lgtm-approve --repo openshift/bpfman-operator | sh

# Use a specific author
./prow-override-test-fmt --author red-hat-konflux --repo openshift/bpfman-operator

# Execute override commands
./prow-override-test-fmt --author red-hat-konflux --repo openshift/bpfman-operator | sh
```

## Systemd Management

Use the `setup-systemd` script to manage the automated services:

```bash
# Install and start all systemd timers
./setup-systemd install

# Check status of all timers and services
./setup-systemd status

# Remove all timers and services
./setup-systemd remove
```

### Monitoring

```bash
# View logs for a specific service
journalctl --user -u prow-ok2test-lgtm-approve-openshift-bpfman-operator -f

# List all active timers
systemctl --user list-timers

# Check status of a specific timer
systemctl --user status prow-ok2test-lgtm-approve-openshift-bpfman-operator.timer
```

## Files

- **`prow-ok2test-lgtm-approve`**: Manual convenience script for LGTM/approve/ok-to-test
- **`prow-override-test-fmt`**: Manual convenience script for overriding test-fmt failures  
- **`setup-systemd`**: Management script that creates systemd units per repository
- Generated systemd files: `prow-{service-type}-{repo-slug}.{service,timer}`