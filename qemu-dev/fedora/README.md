# Fedora Development Setup for bpfman

This directory contains Ansible playbooks for setting up bpfman development environments on Fedora systems.

## Files

- `bpfman-dev-setup.yml` - Main playbook for installing bpfman development dependencies
- `inventory.ini` - Example inventory file for target systems

## Usage

### Local Installation

```bash
# Install on localhost
ansible-playbook -i localhost, -c local bpfman-dev-setup.yml

# With sudo password prompt
ansible-playbook -i localhost, -c local --ask-become-pass bpfman-dev-setup.yml
```

### Remote VM Installation

```bash
# Setup inventory file with your VM details
cp inventory.ini.example inventory.ini
# Edit inventory.ini with your VM IP/hostname

# Run playbook against remote systems
ansible-playbook -i inventory.ini bpfman-dev-setup.yml

# With SSH key authentication
ansible-playbook -i inventory.ini --private-key ~/.ssh/id_rsa bpfman-dev-setup.yml
```

### QEMU VM Integration

This playbook is designed to work with the `bpfman-dev-qemu.sh` script:

```bash
# Start VM
./scripts/bpfman-dev-qemu.sh --cloud-image fedora-cloud.qcow2

# Once VM is running, apply playbook
ansible-playbook -i "VM_IP," -u aim --ask-pass fedora/bpfman-dev-setup.yml
```

## Package Categories

- **Core Build Dependencies**: LLVM, Clang, CMake, Protocol Buffers
- **Runtime Libraries**: OpenSSL, zlib, elfutils
- **Development Tools**: Rust toolchain, debugging tools, container runtime
- **RPM Tools**: Packaging and distribution tools

## Verification

The playbook verifies tool installation and displays version information for key components. Check `/etc/motd.d/bpfman-dev` for environment summary.

## Requirements

- Ansible 2.9 or later
- Target system running Fedora
- SSH access to target (for remote deployment)
- sudo privileges on target system