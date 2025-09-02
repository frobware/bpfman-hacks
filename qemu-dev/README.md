# QEMU Development Tools for bpfman

A collection of tools for developing and testing bpfman in QEMU virtual machines.

## Tools

- **bpfman-dev-qemu**: Create and run a Fedora VM with VirtFS file sharing for bpfman development
- **bpfman-cleanup-integration-test**: Clean up test resources including namespaces, containers, and BPF state
- **bpfman-gen-config**: Generate default bpfman.toml configuration

## Usage

### Local Directory

```bash
# Enter development shell
nix develop

# Run tools directly
./bpfman-dev-qemu --cloud-image fedora-cloud.qcow2
./bpfman-gen-config > /etc/bpfman/bpfman.toml
./bpfman-cleanup-integration-test

# Or via nix run
nix run .#bpfman-dev-qemu -- --cloud-image fedora-cloud.qcow2
nix run .#bpfman-gen-config
nix run .#bpfman-cleanup-integration-test
```

### Remote Usage

```bash
# Run from GitHub repository
nix run github:frobware/bpfman-hacks?dir=qemu-dev#bpfman-dev-qemu -- --cloud-image fedora-cloud.qcow2
nix run github:frobware/bpfman-hacks?dir=qemu-dev#bpfman-gen-config
nix run github:frobware/bpfman-hacks?dir=qemu-dev#bpfman-cleanup-integration-test

# Install tools to your profile
nix profile install github:frobware/bpfman-hacks?dir=qemu-dev#bpfman-dev-qemu
```

## Dependencies

All dependencies are provided automatically via the Nix flake:
- QEMU with KVM support
- virtiofsd for VirtFS
- genisoimage for cloud-init ISO creation
- System utilities (lscpu, realpath)
- Cloud-init tools

## VM Features

The bpfman-dev-qemu tool creates VMs with:
- VirtFS file sharing (host home directory and Nix store)
- Auto-login as current user
- SSH access on port 2222
- BPF-capable kernel
- Full isolation with transparent file access

## Performance Modes

### Safe Mode (default)
```bash
./bpfman-dev-qemu --cloud-image fedora-cloud.qcow2
# or explicitly
FAST_VIRTFS=0 ./bpfman-dev-qemu --cloud-image fedora-cloud.qcow2
```
- Host changes visible immediately in VM
- ~26% build overhead compared to host
- File coherency maintained

### Fast Mode
```bash
FAST_VIRTFS=1 ./bpfman-dev-qemu --cloud-image fedora-cloud.qcow2
```
- ~1% build overhead compared to host (near-native performance)
- **Breaks file coherency** - host changes may not appear in VM
- Use `flush-cache` command in VM to force seeing host changes

### Maximum Performance
```bash
FAST_VIRTFS=1 DISABLE_MITIGATIONS=1 ./bpfman-dev-qemu --cloud-image fedora-cloud.qcow2
```
- Combines aggressive VirtFS caching with disabled CPU mitigations
- Best possible build performance in VM
- Sacrifices both file coherency and security for speed

## Cache Management

When using `FAST_VIRTFS=1`, host file changes may not appear in the VM. To force cache flush in the VM:

```bash
flush-cache
```

This command syncs filesystems and drops VM caches, making host changes visible.