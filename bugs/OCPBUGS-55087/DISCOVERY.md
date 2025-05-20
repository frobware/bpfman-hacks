# Exploring the BPFman Operator Catalog Container Image

This document outlines the process and commands used to explore the BPFman operator catalog container image.

## Initial Setup

Pull the catalog image:

```bash
podman pull quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0
```

## Interactive Container Exploration

### Starting an Interactive Shell Session

Run the container with an interactive shell:

```bash
# Run with bash as the entrypoint (preferred method)
podman run -it --rm --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0
```

### Running and Attaching to a Container

If you want to keep the container's original entrypoint but also want to explore it:

```bash
# Start the container with a name
podman run -d --name bpfman-catalog quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0

# Execute a shell in the running container
podman exec -it bpfman-catalog bash
```

### Creating a Container that Keeps Running

Create a container that stays running with a sleep command, useful if the default entrypoint exits too quickly:

```bash
# Start container with sleep command to keep it running
podman run -d --name bpfman-catalog-sleep --entrypoint sleep quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 3600  # sleeps for 1 hour

# Connect to the running container
podman exec -it bpfman-catalog-sleep bash
```

### Exploring the Container Interactively

Once you're inside the container, you can use standard Linux commands to explore:

```bash
# List all files in the configs directory
ls -la /configs

# Explore the operator package
ls -la /configs/bpfman-operator

# View the catalog file
cat /configs/bpfman-operator/index.yaml

# Explore OPM commands
opm --help

# Render the full catalog content
opm render /configs/bpfman-operator -o yaml

# Check running processes
ps aux

# Look for specific content
find / -name "*bpfman*" 2>/dev/null

# View environment variables
env
```

### Copying Files from Container

If you want to extract files from the container to your host for further analysis:

```bash
# While the container is running
podman cp bpfman-catalog-sleep:/configs/bpfman-operator/index.yaml ./extracted-index.yaml

# Or with a one-liner without keeping the container
podman run --rm --entrypoint cat quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 /configs/bpfman-operator/index.yaml > extracted-index.yaml
```

## Inspection Commands

### Basic Image Information

Get detailed information about the container image:

```bash
podman inspect quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0
```

This command provides:
- Creation date (May 19, 2025)
- Container configuration and environment variables
- Exposed ports (50051/tcp)
- Entry point (/bin/opm)
- Default command (serve /configs --cache-dir=/tmp/cache)
- Image layers and history
- Labels containing metadata about the image

### Exploring Image Contents

To examine the file structure inside the image, create a container and export its filesystem:

```bash
# Create a container without running it
podman create --name bpfman-catalog quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0

# Export the container's filesystem and search for relevant files
podman export bpfman-catalog | tar -tvf - | grep -i -E '(bpfman|config)'
```

### Examining Specific Files

To list the contents of the operator directory:

```bash
podman run --rm -it --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 -c "ls -la /configs/bpfman-operator"
```

To view the beginning of the catalog index file:

```bash
podman run --rm -it --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 -c "cat /configs/bpfman-operator/index.yaml | head -50"
```

To search for specific sections in the index file:

```bash
podman run --rm -it --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 -c "cat /configs/bpfman-operator/index.yaml | grep -A5 'alm-examples'"
```

### Using OPM Tool

To list the OPM binary in the image:

```bash
podman run --rm -it --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 -c "ls -la /bin/ | grep opm"
```

To render the operator catalog content:

```bash
podman run --rm -it --entrypoint bash quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0 -c "opm render /configs/bpfman-operator -o yaml | head -30"
```

## Findings

The image contains:

1. The OPM (Operator Package Manager) tool used to serve the catalog
2. A bpfman-operator bundle (version 0.5.6)
3. Configuration in `/configs/bpfman-operator/index.yaml`

The catalog defines several Custom Resource Definitions:
- BpfApplication
- BpfApplicationState
- ClusterBpfApplication
- ClusterBpfApplicationState

The image is built on top of a Red Hat operator registry base image for OpenShift 4.19, and includes the necessary tools to serve the operator catalog to a Kubernetes cluster.

## Cleanup

Remove containers created for inspection:

```bash
# Remove specific containers
podman rm bpfman-catalog
podman rm bpfman-catalog-sleep

# Or remove all stopped containers
podman container prune

# Force remove running containers
podman rm -f bpfman-catalog-sleep

# Remove the image when done
podman rmi quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0
```