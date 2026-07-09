# Remove `oc` Binary and Library Dependency from oadp-must-gather

**Date:** 2025-07-09
**Status:** Proposed

## Problem

The oadp-must-gather container image ships the `oc` CLI binary (~120MB) from a separate `ose-cli` base image stage. The Go codebase also imports `github.com/openshift/oc` as a library dependency for the `oc adm inspect` functionality. This creates:

1. **A large container image** -- the `oc` binary is significant and never invoked as a subprocess
2. **A heavy Go module dependency** -- `github.com/openshift/oc` pulls in a large transitive dependency tree
3. **Build coupling** -- downstream Dockerfiles depend on the `ose-cli` base image, which must be version-tracked separately
4. **FIPS complexity** -- the `oc` binary must be built with FIPS compliance, adding another build artifact to manage

## Current State

### How `oc` is used today

**Binary:** The `oc` binary is copied into the container via `COPY --from=ose-cli /usr/bin/oc /usr/bin/oc` in `Dockerfile.oadp` and `konflux.Dockerfile`. It is **never** exec'd as a subprocess -- zero `exec.Command("oc"...)` calls exist in the Go code.

**Library:** The Go code imports `github.com/openshift/oc/pkg/cli/admin/inspect` and uses it at a single call site in `pkg/cli.go` (lines 298-320):

```go
ocAdmInspect := ocadminspect.NewInspectOptions(genericiooptions.NewTestIOStreamsDiscard())
ocAdmInspect.DestDir = outputPath
ocAdmInspectNamespaces := []string{}
for namespace := range importantCSVsByNamespace {
    ocAdmInspectNamespaces = append(ocAdmInspectNamespaces, "ns/"+namespace)
}
err = ocAdmInspect.Complete(ocAdmInspectNamespaces)
err = ocAdmInspect.Validate()
err = ocAdmInspect.Run()
```

This call collects comprehensive namespace data for OADP-related namespaces: all Kubernetes resource types, pod logs (current + previous), events, and secrets (with data redacted).

### What `oc adm inspect` does for a namespace

When called with `ns/<namespace>`, the inspect library:

1. Writes the Namespace object as YAML
2. Lists pods and gathers per-pod data (YAML + container logs for all containers and init containers)
3. Collects these resource types: `configmaps`, `egressfirewalls`, `egressqoses`, `events`, `endpoints`, `endpointslices`, `secrets` (data redacted), `persistentvolumeclaims`, `poddisruptionbudgets`, `networkpolicies`, `servicemonitors`, and the "all" pseudo-resource (pods, deployments, replicasets, services, daemonsets, statefulsets, replicationcontrollers, horizontalpodautoscalers, cronjobs, jobs)
4. Writes everything to a structured directory layout under `namespaces/<ns>/`

## Solution: Native Go Implementation

Replace the `oc` library usage with a new `pkg/inspect/` package that implements the same namespace data collection using `k8s.io/client-go` (already a dependency).

### New Package: `pkg/inspect/`

#### Files

| File | Purpose |
|------|---------|
| `inspect.go` | Entry point: `InspectNamespaces(config *rest.Config, destDir string, namespaces []string) error` |
| `namespace.go` | Namespace resource enumeration and collection |
| `pod.go` | Pod log collection (current + previous for all containers) |
| `secret.go` | Secret data sanitization (replace `.data` values with `"redacted"`) |
| `writer.go` | YAML file writer using `k8s.io/cli-runtime/pkg/printers` |

#### Entry Point

```go
package inspect

import "k8s.io/client-go/rest"

func InspectNamespaces(restConfig *rest.Config, destDir string, namespaces []string) error
```

Creates a `kubernetes.Interface` and `dynamic.Interface` from the provided REST config (reusing the same QPS/Burst settings already set by the caller), then iterates over each namespace.

#### Namespace Resource Collection

For each namespace, the code:

1. Gets the Namespace object and writes it as `namespaces/<ns>/<ns>.yaml`
2. For each resource type in the hardcoded list (matching `oc adm inspect`'s `namespaceResourcesToCollect()`):
   - Uses `dynamic.Interface.Resource(gvr).Namespace(ns).List(...)` to list all instances
   - Writes the list as YAML to `namespaces/<ns>/<group>/<resource>.yaml`
   - For secrets, sanitizes data before writing
3. Lists pods and gathers pod-level data

The resource types to collect:

| Resource | API Group | Notes |
|----------|-----------|-------|
| pods | core/v1 | Also triggers per-pod log collection |
| replicationcontrollers | core/v1 | Part of "all" |
| services | core/v1 | Part of "all" |
| daemonsets | apps/v1 | Part of "all" |
| deployments | apps/v1 | Part of "all" |
| replicasets | apps/v1 | Part of "all" |
| statefulsets | apps/v1 | Part of "all" |
| horizontalpodautoscalers | autoscaling/v2 | Part of "all" |
| cronjobs | batch/v1 | Part of "all" |
| jobs | batch/v1 | Part of "all" |
| configmaps | core/v1 | |
| events | core/v1 | |
| endpoints | core/v1 | |
| endpointslices | discovery.k8s.io/v1 | |
| secrets | core/v1 | Data values redacted |
| persistentvolumeclaims | core/v1 | |
| poddisruptionbudgets | policy/v1 | |
| networkpolicies | networking.k8s.io/v1 | |
| egressfirewalls | k8s.ovn.org/v1 | OVN-Kubernetes; may not exist; skip gracefully |
| egressqoses | k8s.ovn.org/v1 | OVN-Kubernetes; may not exist; skip gracefully |
| servicemonitors | monitoring.coreos.com/v1 | May not exist; skip gracefully |

#### Pod Log Collection

For each pod in the namespace:

1. Write pod YAML to `namespaces/<ns>/pods/<pod-name>/<pod-name>.yaml`
2. For each container (regular + init):
   - Collect current logs: `namespaces/<ns>/pods/<pod-name>/<container>/<container>/logs/current.log`
   - Collect previous logs: `namespaces/<ns>/pods/<pod-name>/<container>/<container>/logs/previous.log`
   - If current log collection fails, retry with `InsecureSkipTLSVerifyBackend: true` writing to `current.insecure.log`
   - If previous logs are unavailable (container never restarted), log the error and continue

Log collection uses `kubernetes.Interface.CoreV1().Pods(ns).GetLogs(name, opts)` with `Timestamps: true`.

#### Secret Sanitization

Before writing secrets to disk, replace each value in `.data` with `"N bytes long"` (where N is the original byte length), preserving public keys (`tls.crt`, `ca.crt`, `service-ca.crt`). Also clear the `openshift.io/token-secret.value` and `kubectl.kubernetes.io/last-applied-configuration` annotations. This matches `oc adm inspect` behavior.

#### Directory Structure

The output matches `oc adm inspect` exactly:

```
<destDir>/
  namespaces/<ns>/
    <ns>.yaml
    core/
      configmaps.yaml
      endpoints.yaml
      events.yaml
      persistentvolumeclaims.yaml
      pods.yaml
      replicationcontrollers.yaml
      secrets.yaml
      services.yaml
    apps/
      daemonsets.yaml
      deployments.yaml
      replicasets.yaml
      statefulsets.yaml
    autoscaling/
      horizontalpodautoscalers.yaml
    batch/
      cronjobs.yaml
      jobs.yaml
    discovery.k8s.io/
      endpointslices.yaml
    networking.k8s.io/
      networkpolicies.yaml
    policy/
      poddisruptionbudgets.yaml
    k8s.ovn.org/
      egressfirewalls.yaml
      egressqoses.yaml
    monitoring.coreos.com/
      servicemonitors.yaml
    pods/<pod-name>/
      <pod-name>.yaml
      <container-name>/<container-name>/logs/
        current.log
        previous.log
```

#### Error Handling

- Resource types that don't exist in the cluster (e.g., `servicemonitors` without Prometheus operator) are skipped gracefully with a log message
- Pod log errors for containers without previous logs are logged but don't stop collection
- All errors are aggregated and returned as a single error at the end, matching `oc adm inspect` behavior

### Integration Changes

#### `pkg/cli.go`

Replace the `oc adm inspect` call block (lines 298-320) with:

```go
if len(importantCSVsByNamespace) != 0 {
    namespacesToInspect := []string{}
    for namespace := range importantCSVsByNamespace {
        namespacesToInspect = append(namespacesToInspect, namespace)
    }
    err = inspect.InspectNamespaces(clusterConfig, outputPath, namespacesToInspect)
    if err != nil {
        fmt.Println(err)
    }
}
```

Remove imports:
- `ocadminspect "github.com/openshift/oc/pkg/cli/admin/inspect"`
- `"k8s.io/cli-runtime/pkg/genericiooptions"` (verify not used elsewhere first)

Add import:
- `"github.com/openshift/oadp-must-gather/pkg/inspect"`

#### `go.mod`

Remove:
- `github.com/openshift/oc v0.0.0-alpha.0.0.20250108103617-ae1bd9e4a75b`

Run `go mod tidy` to remove cascading unused transitive dependencies.

Verify that `k8s.io/client-go`, `k8s.io/apimachinery`, and `k8s.io/cli-runtime` remain (they're used elsewhere).

#### `Dockerfile.oadp`

Remove the `ose-cli` stage:
```dockerfile
# REMOVE THIS LINE:
FROM brew.registry.redhat.io/rh-osbs/openshift-ose-cli-rhel9:v4.19 AS ose-cli

# REMOVE THIS LINE:
COPY --from=ose-cli /usr/bin/oc /usr/bin/oc
```

#### `konflux.Dockerfile`

Remove the `ose-cli` stage:
```dockerfile
# REMOVE THIS LINE:
FROM brew.registry.redhat.io/rh-osbs/openshift-ose-cli-rhel9:v4.21 AS ose-cli

# REMOVE THIS LINE:
COPY --from=ose-cli /usr/bin/oc /usr/bin/oc
```

#### `README.md`

Remove the section about updating the `oc` dependency:
```
Possible necessary updates over the time
go get github.com/openshift/oc@<branch-or-commit>
go mod tidy
go mod verify
```

### Testing Strategy

#### Unit Tests

- `pkg/inspect/secret_test.go` -- verify secret sanitization correctly redacts `.data` values while preserving `.metadata` and `.type`
- `pkg/inspect/writer_test.go` -- verify YAML output format matches expected structure

#### Integration Validation

1. Build the container locally using `Dockerfile`:
   ```sh
   podman build -t ttl.sh/oadp/must-gather-test:1h -f Dockerfile . --platform=linux/amd64
   ```

2. Run against a test cluster and compare output with a known-good `oc adm must-gather` run:
   - Same directory tree under `namespaces/<ns>/`
   - Same YAML format for each resource type
   - Pod logs present (`current.log`, `previous.log`)
   - Secrets have `.data` values redacted

3. Verify `omg` tool compatibility:
   ```sh
   omg use must-gather/clusters/
   omg get backup -n <namespace>
   ```

4. Run existing OADP E2E tests that validate must-gather output

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Directory layout mismatch breaks `omg` | Exact-match directory structure with `oc adm inspect`; validate with `omg` before merge |
| Missing resource types in collection | Hardcode the same list as `oc adm inspect`; add discovery-based fallback if a resource type is not found |
| FIPS compliance regression | No change to FIPS: we're removing a binary, not adding one; the Go code already builds with `strictfipsruntime` |
| `k8s.io/cli-runtime` version drift after removing `oc` | Pin version explicitly in `go.mod`; it's already a direct dependency |

### Benefits

1. **Smaller container image** -- removes ~120MB `oc` binary
2. **Simpler dependency tree** -- removes `github.com/openshift/oc` and its ~50+ transitive dependencies
3. **Faster builds** -- no need to pull the `ose-cli` base image stage
4. **Reduced CVE surface** -- fewer dependencies = fewer potential CVEs to track
5. **Self-contained** -- must-gather owns all its collection logic, no external binary coupling
