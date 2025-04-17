# Installing bpfman Operator via FBC on OpenShift

https://issues.redhat.com/browse/OCPBUGS-55087

This guide documents the reproducible, YAML-based steps used to install the `bpfman` Operator in the `bpfman-test` namespace on OpenShift, using a File-Based Catalog (FBC) pinned to a specific image digest.

## Files Used

```text
-rw-r--r-- 1 aim aim 50351 Apr 17 16:29 bpfman-catalog.yaml
-rw-r--r-- 1 aim aim   740 Apr 17 16:40 bpfman-idms.yaml
-rw-r--r-- 1 aim aim   410 Apr 17 16:43 bpfman-catalogsource.yaml
-rw-r--r-- 1 aim aim   341 Apr 17 17:03 subscription.yaml
-rw-r--r-- 1 aim aim   207 Apr 17 17:27 operator-group.yaml
```

## YAML Install Order

0. **Namespace**

   ```bash
   oc new-project bpfman-test
   ```

1. **ImageDigestMirrorSet (IDMS)**

   Apply `bpfman-idms.yaml` to ensure image mirroring:

   ```bash
   oc apply -f bpfman-idms.yaml
   ```

2. **Catalog Rendering**

   Render the FBC catalog image and save as `bpfman-catalog.yaml`:

   ```bash
   opm render quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19@sha256:92697178cbb3ae4883a72b378bc21200485d5171e0f2db5647c53a0ebbca3d06 \
     --output=yaml > bpfman-catalog.yaml

   oc apply -f bpfman-catalog.yaml
   ```

3. **CatalogSource**

   Apply the catalog source that references the FBC contents:

   ```bash
   oc apply -f bpfman-catalogsource.yaml
   ```

4. **OperatorGroup**

   Ensure the `operator-group.yaml` is compatible with the install modes declared in the CSV. In this case, only `AllNamespaces` is supported:

   ```yaml
   apiVersion: operators.coreos.com/v1
   kind: OperatorGroup
   metadata:
     name: bpfman-test
     namespace: bpfman-test
   spec:
     targetNamespaces:
       - bpfman-test
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
   $ oc get csv -n bpfman-test
   NAME                                DISPLAY                                       VERSION   REPLACES                            PHASE
   bpfman-operator.v0.5.6              eBPF Manager Operator                         0.5.6                                         Succeeded
   cert-manager-operator.v1.15.1       cert-manager Operator for Red Hat OpenShift   1.15.1    cert-manager-operator.v1.15.0       Succeeded
   security-profiles-operator.v0.8.6   Security Profiles Operator                    0.8.6     security-profiles-operator.v0.8.5   Succeeded
   ```
   
   ```bash
   % oc get pods -n bpfman-test
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
   
