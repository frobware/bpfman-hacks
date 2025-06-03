# OpenShift Metrics Test

This directory contains standalone tests for validating bpfman-operator metrics collection in OpenShift clusters.

## Prerequisites

- OpenShift cluster with bpfman-operator deployed using the OpenShift configuration
- OpenShift monitoring stack enabled (prometheus-k8s service account available)
- kubectl/oc access to the cluster with appropriate permissions

## Running the Tests

```bash
# Run all OpenShift tests
go test -tags openshift -v

# Run specific test
go test -tags openshift -v -run TestOpenShiftAgentMetricsCollection
go test -tags openshift -v -run TestOpenShiftPrometheusAccess

# From the project root
go test -tags openshift -v ./cmd/openshift/
```

## Test Suite

### TestOpenShiftAgentMetricsCollection

Validates the complete metrics collection pipeline from the metrics-proxy pod perspective:

1. **Health endpoint** - Tests `/healthz` on port 8081 (HTTP, no auth)
2. **Direct Unix socket access** - Tests the bpfman-agent metrics server via Unix socket
3. **Localhost HTTPS proxy** - Tests `/agent-metrics` on port 8443 with authentication (uses `-k` for certificate bypass due to localhost vs service DNS name mismatch)

### TestOpenShiftPrometheusAccess

Validates end-to-end metrics access from the Prometheus pod perspective:

1. **Service DNS access** - Tests `bpfman-agent-metrics-service.bpfman.svc.cluster.local:8443/agent-metrics`
2. **Proper certificate validation** - Uses CA certificate bundles that Prometheus actually uses
3. **Production authentication** - Uses the `prometheus-k8s` service account token
4. **Real monitoring scenario** - Simulates exactly how Prometheus scrapes metrics in production

## Certificate Validation Strategy

- **Metrics-proxy tests**: Use `-k` flag for localhost access (certificates issued for service DNS names)
- **Prometheus tests**: Use proper CA certificate validation with service DNS names
- **Fallback support**: Prometheus tests can fall back to service account CA if needed

## Test Output

Both test suites validate:
- Health check endpoint returns "ok" 
- Unix socket serves ~28KB of Prometheus-formatted metrics
- HTTPS endpoints return equivalent metrics data with proper authentication
- Certificate validation works correctly for each access pattern

## What These Tests Prove

These tests validate that the Unix socket metrics server lifecycle improvements work correctly in production OpenShift:

- **Resource cleanup** - Socket files are properly removed on shutdown
- **Graceful shutdown** - Context cancellation triggers proper server shutdown
- **Error handling** - Server failures are handled gracefully without resource leaks
- **Production integration** - Real Prometheus can access metrics using standard OpenShift patterns
- **Certificate management** - Both localhost and service DNS access patterns work correctly

## Troubleshooting

If tests fail:

1. **Check namespace**: Ensure bpfman-operator is deployed in the `bpfman` namespace
2. **Verify monitoring**: Confirm OpenShift monitoring is enabled and `prometheus-k8s` SA exists
3. **Check RBAC**: Verify the `bpfman-prometheus-metrics-reader` ClusterRoleBinding is applied
4. **Pod status**: Ensure both `bpfman-metrics-proxy` and `prometheus-k8s` pods are running
5. **Service status**: Verify `bpfman-agent-metrics-service` exists and has endpoints

## Differences from Standard Integration Test

This OpenShift-specific test suite:
- Uses existing OpenShift monitoring infrastructure instead of creating temporary resources
- Tests from both client (Prometheus) and server (metrics-proxy) perspectives  
- Uses production-realistic certificate validation patterns
- Validates the complete end-to-end metrics collection flow
- Runs fewer iterations since OpenShift environments are typically more stable