# bpfman Operator Install via FBC

https://issues.redhat.com/browse/OCPBUGS-55087

Steps to install bpfman Operator in the `bpfman` namespace using File-Based Catalog (FBC). You must specify a catalog image digest.

Available catalog images can be found at [Quay.io Repository](https://quay.io/repository/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19?tab=tags).

## Quick Start

```bash
# Install using justfile (install 'just' command if needed)
just install quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:db0d7261b555a9bbf65a864ff0f72e3425d1ae2f37bfe02d6975b81fc6ee6ba0

# Check status
just check-status
```

## Files

```text
bpfman-idms.yaml            # ImageDigestMirrorSet config
bpfman-catalogsource.yaml   # CatalogSource template (needs image)
subscription.yaml           # Operator subscription
operator-group.yaml         # OperatorGroup config
justfile                    # Automation script
image-dates.md              # Available catalog images info
```

## Manual Install Steps

> **Note:** These manual steps are for reference. Use the justfile for easier installation.

0. **Namespace**

   ```bash
   oc new-project bpfman
   ```

1. **ImageDigestMirrorSet (IDMS)**

   Apply `bpfman-idms.yaml` to ensure image mirroring:

   ```bash
   oc apply -f bpfman-idms.yaml
   ```

2. **CatalogSource**

   Create the catalog source:

   ```bash
   just --set image=quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:DIGEST create-catalogsource
   ```

   > The CatalogSource template has no hard-coded image reference. You must provide the image digest.

4. **OperatorGroup**

   Ensure the `operator-group.yaml` is compatible with the install modes declared in the CSV. In this case, only `AllNamespaces` is supported:

   ```yaml
   apiVersion: operators.coreos.com/v1
   kind: OperatorGroup
   metadata:
     name: bpfman
     namespace: bpfman
   spec:
     targetNamespaces:
       - bpfman
   ```

   ```bash
   oc apply -f operator-group.yaml
   ```

5. **Subscription**

   Subscribe to the operator via:

   ```bash
   oc apply -f subscription.yaml
   ```

## Results

   ```bash
   $ oc get csv -n bpfman
   NAME                                DISPLAY                                       VERSION   REPLACES                            PHASE
   bpfman-operator.v0.5.6              eBPF Manager Operator                         0.5.6                                         Succeeded
   cert-manager-operator.v1.15.1       cert-manager Operator for Red Hat OpenShift   1.15.1    cert-manager-operator.v1.15.0       Succeeded
   security-profiles-operator.v0.8.6   Security Profiles Operator                    0.8.6     security-profiles-operator.v0.8.5   Succeeded
   ```

   ```bash
   % oc get pods -n bpfman
   NAME                                                  READY   STATUS    RESTARTS   AGE
   bpfman-operator-c6c99d787-mw5t8                       2/2     Running   0          22m
   security-profiles-operator-684cbc68d7-pq9r2           1/1     Running   0          22m
   security-profiles-operator-684cbc68d7-shm4d           1/1     Running   0          22m
   security-profiles-operator-684cbc68d7-vtvkt           1/1     Running   0          22m
   security-profiles-operator-webhook-5c6bb6948c-99zrv   1/1     Running   0          45m
   security-profiles-operator-webhook-5c6bb6948c-cvq29   1/1     Running   0          45m
   security-profiles-operator-webhook-5c6bb6948c-tghqx   1/1     Running   0          45m
   spod-cn287                                            3/3     Running   0          45m
   spod-fscqd                                            3/3     Running   0          45m
   spod-nhnv6                                            3/3     Running   0          45m
   ```

## Justfile Commands {#using-the-justfile}

The `justfile` automates installation steps. [Just](https://github.com/casey/just) is a command runner tool.

### Main Commands

```bash
# Install the operator (image reference required)
just install quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:DIGEST

# Show image info
just show-image-info quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:DIGEST

# Update to a different image
just update-catalog-image quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:NEW_DIGEST


# Check installation status
just check-status
```

For example image references, see [image-dates.md](./image-dates.md).

### Step-by-Step Commands

```bash
# Individual installation steps
just create-namespace
just apply-idms
just create-catalogsource IMAGE_REFERENCE           # image required
just create-operatorgroup
just create-subscription

# Other operations
just check-status                                   # Check installation status
just cleanup                                        # Remove all components
```

### Cleanup

```bash
just cleanup  # Remove everything
```

