# Remove `oc` Dependency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `oc` binary and `github.com/openshift/oc` Go library from oadp-must-gather by replacing `oc adm inspect` with native Go code using `k8s.io/client-go`.

**Architecture:** A new `pkg/inspect/` package replaces the single `oc adm inspect` call site in `pkg/cli.go`. It uses `k8s.io/client-go/kubernetes` for pod logs and `k8s.io/client-go/dynamic` for resource listing, writing to the exact same directory layout. The `oc` binary and its Dockerfile stage are then removed.

**Tech Stack:** Go, `k8s.io/client-go`, `k8s.io/apimachinery`, `k8s.io/cli-runtime/pkg/printers`

**Spec:** `docs/superpowers/specs/2025-07-09-remove-oc-dependency-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `pkg/inspect/inspect.go` | Entry point: `InspectNamespaces`, client creation |
| Create | `pkg/inspect/namespace.go` | Namespace resource listing, directory layout, resource GVR table |
| Create | `pkg/inspect/pod.go` | Pod log collection (current + previous for all containers) |
| Create | `pkg/inspect/secret.go` | Secret data elision (match upstream `oc` logic exactly) |
| Create | `pkg/inspect/secret_test.go` | Unit tests for secret elision |
| Create | `pkg/inspect/writer.go` | YAML file writer, log file writer |
| Modify | `pkg/cli.go:1-28,298-320` | Replace oc import + call site with `inspect.InspectNamespaces` |
| Modify | `go.mod:13` | Remove `github.com/openshift/oc` dependency |
| Modify | `Dockerfile.oadp:3-5,60` | Remove `ose-cli` stage and `COPY --from=ose-cli` |
| Modify | `konflux.Dockerfile:1-2,55` | Remove `ose-cli` stage and `COPY --from=ose-cli` |
| Modify | `README.md:60-65` | Remove `oc` update instructions |

---

### Task 1: Create `pkg/inspect/writer.go` — YAML and log file writer

**Files:**
- Create: `pkg/inspect/writer.go`

This is the lowest-level utility. All other files depend on it.

- [ ] **Step 1: Create `pkg/inspect/writer.go`**

```go
package inspect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/rest"
)

const (
	filePermission   = 0644
	folderPermission = os.ModePerm
)

func writeYAML(filepath string, obj runtime.Object) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer f.Close()

	printer := printers.YAMLPrinter{}
	return printer.PrintObj(obj, f)
}

func writeLogFromRequest(filepath string, req *rest.Request) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	stream, err := req.Stream(context.TODO())
	if err != nil {
		return fmt.Errorf("unable to stream logs to %s: %w", filepath, err)
	}
	defer stream.Close()

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, stream)
	return err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./pkg/inspect/`
Expected: No errors (package has no dependencies on other new files yet)

- [ ] **Step 3: Commit**

```bash
git add pkg/inspect/writer.go
git commit -m "feat(inspect): add YAML and log file writer utilities"
```

---

### Task 2: Create `pkg/inspect/secret.go` — Secret data elision

**Files:**
- Create: `pkg/inspect/secret.go`
- Create: `pkg/inspect/secret_test.go`

This replicates the upstream `oc adm inspect` secret sanitization exactly. Public cert keys (`tls.crt`, `ca.crt`, `service-ca.crt`) are preserved. Other data values are replaced with `"N bytes long"`. Sensitive annotations are cleared.

- [ ] **Step 1: Write the test file `pkg/inspect/secret_test.go`**

```go
package inspect

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestElideSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "openshift-adp",
			Annotations: map[string]string{
				"openshift.io/token-secret.value":                  "sensitive-token",
				"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"Secret"}`,
				"safe-annotation": "keep-this",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte("super-secret-password"),
			"tls.crt":  []byte("-----BEGIN CERTIFICATE-----"),
			"ca.crt":   []byte("-----BEGIN CERTIFICATE-----"),
			"service-ca.crt": []byte("-----BEGIN CERTIFICATE-----"),
			"api-key":  []byte("abc123"),
		},
	}

	elideSecret(secret)

	if string(secret.Data["password"]) != "21 bytes long" {
		t.Errorf("expected password to be '21 bytes long', got %q", string(secret.Data["password"]))
	}
	if string(secret.Data["api-key"]) != "6 bytes long" {
		t.Errorf("expected api-key to be '6 bytes long', got %q", string(secret.Data["api-key"]))
	}
	if string(secret.Data["tls.crt"]) != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("expected tls.crt to be preserved, got %q", string(secret.Data["tls.crt"]))
	}
	if string(secret.Data["ca.crt"]) != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("expected ca.crt to be preserved, got %q", string(secret.Data["ca.crt"]))
	}
	if string(secret.Data["service-ca.crt"]) != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("expected service-ca.crt to be preserved, got %q", string(secret.Data["service-ca.crt"]))
	}
	if secret.Annotations["openshift.io/token-secret.value"] != "" {
		t.Errorf("expected token annotation to be cleared, got %q", secret.Annotations["openshift.io/token-secret.value"])
	}
	if secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"] != "" {
		t.Errorf("expected last-applied annotation to be cleared, got %q", secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	}
	if secret.Annotations["safe-annotation"] != "keep-this" {
		t.Errorf("expected safe-annotation to be preserved, got %q", secret.Annotations["safe-annotation"])
	}
}

func TestElideSecretNilData(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "empty"},
		Data:       nil,
	}
	elideSecret(secret)
	if secret.Data != nil {
		t.Error("expected nil data to remain nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go test ./pkg/inspect/ -run TestElideSecret -v`
Expected: FAIL — `elideSecret` not defined

- [ ] **Step 3: Create `pkg/inspect/secret.go`**

```go
package inspect

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var publicSecretKeys = sets.NewString(
	"tls.crt",
	"ca.crt",
	"service-ca.crt",
)

func elideSecret(secret *corev1.Secret) {
	for k, v := range secret.Data {
		if publicSecretKeys.Has(k) {
			continue
		}
		secret.Data[k] = []byte(fmt.Sprintf("%d bytes long", len(v)))
	}

	if _, ok := secret.Annotations["openshift.io/token-secret.value"]; ok {
		secret.Annotations["openshift.io/token-secret.value"] = ""
	}
	if _, ok := secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = ""
	}
}

func elideSecretList(list *corev1.SecretList) {
	for i := range list.Items {
		elideSecret(&list.Items[i])
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go test ./pkg/inspect/ -run TestElideSecret -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/inspect/secret.go pkg/inspect/secret_test.go
git commit -m "feat(inspect): add secret data elision matching oc adm inspect"
```

---

### Task 3: Create `pkg/inspect/pod.go` — Pod log collection

**Files:**
- Create: `pkg/inspect/pod.go`

Collects current and previous container logs for all containers and init containers in a pod. Retries with `InsecureSkipTLSVerifyBackend: true` on failure. Writes to the same path structure as `oc adm inspect`.

- [ ] **Step 1: Create `pkg/inspect/pod.go`**

```go
package inspect

import (
	"fmt"
	"path"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
)

func gatherPodData(kubeClient kubernetes.Interface, destDir string, pod *corev1.Pod) error {
	podDir := path.Join(destDir, pod.Name)

	if err := writeYAML(path.Join(podDir, pod.Name+".yaml"), pod); err != nil {
		return err
	}

	var errs []error
	for _, container := range pod.Spec.Containers {
		if err := gatherContainerLogs(kubeClient, podDir, pod, container); err != nil {
			errs = append(errs, err)
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if err := gatherContainerLogs(kubeClient, podDir, pod, container); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}

func gatherContainerLogs(kubeClient kubernetes.Interface, podDir string, pod *corev1.Pod, container corev1.Container) error {
	logsDir := path.Join(podDir, container.Name, container.Name, "logs")

	var errs []error
	wg := sync.WaitGroup{}
	errLock := sync.Mutex{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		var innerErrs []error

		logOpts := &corev1.PodLogOptions{
			Container:  container.Name,
			Follow:     false,
			Previous:   false,
			Timestamps: true,
		}
		req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
		if err := writeLogFromRequest(path.Join(logsDir, "current.log"), req); err != nil {
			innerErrs = append(innerErrs, err)
			logOpts.InsecureSkipTLSVerifyBackend = true
			req = kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
			if err := writeLogFromRequest(path.Join(logsDir, "current.insecure.log"), req); err != nil {
				innerErrs = append(innerErrs, err)
			}
		}

		errLock.Lock()
		defer errLock.Unlock()
		errs = append(errs, innerErrs...)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var innerErrs []error

		logOpts := &corev1.PodLogOptions{
			Container:  container.Name,
			Follow:     false,
			Previous:   true,
			Timestamps: true,
		}
		req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
		if err := writeLogFromRequest(path.Join(logsDir, "previous.log"), req); err != nil {
			if !isPreviousContainerNotFound(err) {
				innerErrs = append(innerErrs, err)
				logOpts.InsecureSkipTLSVerifyBackend = true
				req = kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
				if err := writeLogFromRequest(path.Join(logsDir, "previous.insecure.log"), req); err != nil {
					innerErrs = append(innerErrs, err)
				}
			}
		}

		errLock.Lock()
		defer errLock.Unlock()
		errs = append(errs, innerErrs...)
	}()

	wg.Wait()
	return errors.NewAggregate(errs)
}

func isPreviousContainerNotFound(err error) bool {
	return strings.Contains(err.Error(), "previous terminated container") &&
		strings.HasSuffix(err.Error(), "not found")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./pkg/inspect/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/inspect/pod.go
git commit -m "feat(inspect): add pod log collection for all containers"
```

---

### Task 4: Create `pkg/inspect/namespace.go` — Namespace resource collection

**Files:**
- Create: `pkg/inspect/namespace.go`

Contains the resource type table (matching `oc adm inspect` exactly), namespace resource listing using dynamic client, and directory path mapping.

- [ ] **Step 1: Create `pkg/inspect/namespace.go`**

```go
package inspect

import (
	"context"
	"fmt"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type namespaceResource struct {
	gvr      schema.GroupVersionResource
	dirGroup string
}

func namespacedResourcesToCollect() []namespaceResource {
	return []namespaceResource{
		// "all" pseudo-resource expansion
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "replicationcontrollers"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, dirGroup: "apps"},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, dirGroup: "apps"},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, dirGroup: "apps"},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, dirGroup: "apps"},
		{gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, dirGroup: "autoscaling"},
		{gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, dirGroup: "batch"},
		{gvr: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, dirGroup: "batch"},

		// explicitly listed resources
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}, dirGroup: "discovery.k8s.io"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, dirGroup: "core"},
		{gvr: schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}, dirGroup: "policy"},
		{gvr: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, dirGroup: "networking.k8s.io"},
		{gvr: schema.GroupVersionResource{Group: "k8s.ovn.org", Version: "v1", Resource: "egressfirewalls"}, dirGroup: "k8s.ovn.org"},
		{gvr: schema.GroupVersionResource{Group: "k8s.ovn.org", Version: "v1", Resource: "egressqoses"}, dirGroup: "k8s.ovn.org"},
		{gvr: schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"}, dirGroup: "monitoring.coreos.com"},
	}
}

func gatherNamespaceData(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, destDir string, namespace string) error {
	fmt.Printf("Gathering data for ns/%s...\n", namespace)

	nsDir := path.Join(destDir, "namespaces", namespace)
	if err := os.MkdirAll(nsDir, folderPermission); err != nil {
		return err
	}

	ns, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get namespace %s: %w", namespace, err)
	}
	ns.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	if err := writeYAML(path.Join(nsDir, namespace+".yaml"), ns); err != nil {
		return err
	}

	var errs []error

	var podList *unstructured.UnstructuredList
	for _, res := range namespacedResourcesToCollect() {
		list, err := dynamicClient.Resource(res.gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("  skipping %s/%s: %v\n", res.dirGroup, res.gvr.Resource, err)
			continue
		}

		if res.gvr.Resource == "pods" {
			podList = list
		}

		objToPrint := runtime.Object(list)

		if res.gvr.Resource == "secrets" {
			secretList, convErr := unstructuredListToSecretList(list)
			if convErr != nil {
				errs = append(errs, convErr)
			} else {
				elideSecretList(secretList)
				objToPrint = secretList
			}
		}

		filePath := path.Join(nsDir, res.dirGroup, res.gvr.Resource+".yaml")
		if err := writeYAML(filePath, objToPrint); err != nil {
			errs = append(errs, err)
		}
	}

	if podList != nil {
		podsDir := path.Join(nsDir, "pods")
		for _, podUnstr := range podList.Items {
			structuredPod := &corev1.Pod{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUnstr.Object, structuredPod); err != nil {
				errs = append(errs, fmt.Errorf("unable to convert pod %s: %w", podUnstr.GetName(), err))
				continue
			}
			if err := gatherPodData(kubeClient, podsDir, structuredPod); err != nil {
				errs = append(errs, fmt.Errorf("error gathering pod data for %s: %w", structuredPod.Name, err))
			}
		}
	}

	return errors.NewAggregate(errs)
}

func unstructuredListToSecretList(list *unstructured.UnstructuredList) (*corev1.SecretList, error) {
	secretList := &corev1.SecretList{}
	for _, item := range list.Items {
		secret := &corev1.Secret{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, secret); err != nil {
			return nil, err
		}
		secretList.Items = append(secretList.Items, *secret)
	}
	return secretList, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./pkg/inspect/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/inspect/namespace.go
git commit -m "feat(inspect): add namespace resource collection with directory layout"
```

---

### Task 5: Create `pkg/inspect/inspect.go` — Entry point

**Files:**
- Create: `pkg/inspect/inspect.go`

The public entry point that creates clients and orchestrates namespace data collection.

- [ ] **Step 1: Create `pkg/inspect/inspect.go`**

```go
package inspect

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func InspectNamespaces(restConfig *rest.Config, destDir string, namespaces []string) error {
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("unable to create dynamic client: %w", err)
	}

	var errs []error
	for _, ns := range namespaces {
		if err := gatherNamespaceData(kubeClient, dynamicClient, destDir, ns); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while inspecting namespaces:\n    %v", errors.NewAggregate(errs))
	}
	return nil
}
```

- [ ] **Step 2: Verify the full package compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./pkg/inspect/`
Expected: No errors

- [ ] **Step 3: Verify all tests pass**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go test ./pkg/inspect/ -v`
Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/inspect/inspect.go
git commit -m "feat(inspect): add InspectNamespaces entry point"
```

---

### Task 6: Integrate `pkg/inspect/` into `pkg/cli.go` and remove `oc` imports

**Files:**
- Modify: `pkg/cli.go:1-28` (imports)
- Modify: `pkg/cli.go:298-320` (call site)

- [ ] **Step 1: Replace the import block in `pkg/cli.go`**

Remove these two imports:
```go
ocadminspect "github.com/openshift/oc/pkg/cli/admin/inspect"
```
```go
"k8s.io/cli-runtime/pkg/genericiooptions"
```

Add this import:
```go
"github.com/openshift/oadp-must-gather/pkg/inspect"
```

The resulting import block should be:

```go
import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	nac1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	vmfrv1alpha1 "github.com/migtools/oadp-vm-file-restore/api/v1alpha1"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/cobra"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/openshift/oadp-must-gather/pkg/gather"
	"github.com/openshift/oadp-must-gather/pkg/inspect"
	"github.com/openshift/oadp-must-gather/pkg/templates"
)
```

- [ ] **Step 2: Replace the `oc adm inspect` call block (lines 298-320)**

Replace this block:

```go
		// oc adm inspect --dest-dir must-gather/clusters/${clusterID} ns/${ns}
		if len(importantCSVsByNamespace) != 0 {
			ocAdmInspect := ocadminspect.NewInspectOptions(genericiooptions.NewTestIOStreamsDiscard())
			ocAdmInspect.DestDir = outputPath
			ocAdmInspectNamespaces := []string{}
			for namespace := range importantCSVsByNamespace {
				ocAdmInspectNamespaces = append(ocAdmInspectNamespaces, "ns/"+namespace)
			}

			// https://github.com/openshift/oc/blob/ae1bd9e4a75b8ab617a569e5c8e1a0d7285a16f6/pkg/cli/admin/inspect/inspect.go#L108
			err = ocAdmInspect.Complete(ocAdmInspectNamespaces)
			if err != nil {
				fmt.Println(err)
			}
			err = ocAdmInspect.Validate()
			if err != nil {
				fmt.Println(err)
			}
			err = ocAdmInspect.Run()
			if err != nil {
				fmt.Println(err)
			}
		}
```

With:

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

- [ ] **Step 3: Verify the whole project compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add pkg/cli.go
git commit -m "refactor: replace oc adm inspect with native inspect package"
```

---

### Task 7: Remove `github.com/openshift/oc` from `go.mod` and tidy

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Remove the `oc` dependency from `go.mod`**

Delete this line from the `require` block in `go.mod`:

```
github.com/openshift/oc v0.0.0-alpha.0.0.20250108103617-ae1bd9e4a75b
```

- [ ] **Step 2: Run `go mod tidy`**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go mod tidy`
Expected: Success. This will also remove cascading transitive dependencies from `go.sum`.

- [ ] **Step 3: Verify it still compiles**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build ./...`
Expected: No errors

- [ ] **Step 4: Verify `k8s.io/client-go` and `k8s.io/cli-runtime` are still present**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && grep 'k8s.io/client-go\|k8s.io/cli-runtime' go.mod`
Expected: Both should appear (used by `pkg/inspect/` and `pkg/templates/`)

- [ ] **Step 5: Run all tests**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go test ./...`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: remove github.com/openshift/oc dependency from go.mod"
```

---

### Task 8: Remove `oc` binary from Dockerfiles

**Files:**
- Modify: `Dockerfile.oadp`
- Modify: `konflux.Dockerfile`

- [ ] **Step 1: Edit `Dockerfile.oadp`**

Remove lines 1-5 (the `ose-cli` stage and its comments):

```dockerfile
# upstream src https://github.com/openshift/oadp-must-gather/blob/oadp-dev/Dockerfile

# oc
#@follow_tag(registry-proxy.engineering.redhat.com/rh-osbs/openshift-ose-cli-rhel9:v4.19)
FROM brew.registry.redhat.io/rh-osbs/openshift-ose-cli-rhel9:v4.19 AS ose-cli
```

Remove line 60:

```dockerfile
COPY --from=ose-cli /usr/bin/oc /usr/bin/oc
```

The resulting file should start with:

```dockerfile
# upstream src https://github.com/openshift/oadp-must-gather/blob/oadp-dev/Dockerfile

#@follow_tag(registry-proxy.engineering.redhat.com/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25)
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25 AS builder
```

And the COPY section should be:

```dockerfile
COPY --from=builder /workspace/velero/bin/velero /usr/bin/velero
COPY --from=builder /workspace/restic/bin/restic /usr/bin/restic
COPY --from=builder /workspace/kopia/kopia /usr/bin/kopia
COPY --from=builder /workspace/gather /usr/bin/gather
COPY --from=builder /workspace/deprecated/gather_* /usr/bin/
COPY --from=builder /workspace/LICENSE /licenses/
```

- [ ] **Step 2: Edit `konflux.Dockerfile`**

Remove lines 1-2 (the `ose-cli` stage):

```dockerfile
# oc
FROM brew.registry.redhat.io/rh-osbs/openshift-ose-cli-rhel9:v4.21 AS ose-cli
```

Remove line 55:

```dockerfile
COPY --from=ose-cli /usr/bin/oc /usr/bin/oc
```

The resulting file should start with:

```dockerfile
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25 AS builder
```

And the COPY section should be:

```dockerfile
COPY --from=builder /workspace/velero/bin/velero /usr/bin/velero
COPY --from=builder /workspace/restic/bin/restic /usr/bin/restic
COPY --from=builder /workspace/kopia/kopia /usr/bin/kopia
COPY --from=builder /workspace/gather /usr/bin/gather
COPY --from=builder /workspace/deprecated/gather_* /usr/bin/
COPY --from=builder /workspace/LICENSE /licenses/
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile.oadp konflux.Dockerfile
git commit -m "chore: remove oc binary and ose-cli stage from Dockerfiles"
```

---

### Task 9: Update `README.md`

**Files:**
- Modify: `README.md:60-65`

- [ ] **Step 1: Remove the `oc` update instructions from README.md**

Remove these lines (around line 60-65):

```markdown
Possible necessary updates over the time
```sh
go get github.com/openshift/oc@<branch-or-commit>
go mod tidy
go mod verify
```
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: remove oc dependency update instructions from README"
```

---

### Task 10: Final validation

- [ ] **Step 1: Run full build**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go build -o /dev/null ./cmd/main.go`
Expected: Success

- [ ] **Step 2: Run all tests**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && go test ./... -v`
Expected: All pass

- [ ] **Step 3: Verify `oc` is fully removed**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && grep -r 'openshift/oc' go.mod pkg/ cmd/ Dockerfile.oadp konflux.Dockerfile`
Expected: No matches

- [ ] **Step 4: Verify container image builds (upstream Dockerfile)**

Run: `cd /home/whayutin/OPENSHIFT/git/OADP/oadp-must-gather && podman build -t oadp-must-gather-test:latest -f Dockerfile . --platform=linux/amd64`
Expected: Successful build

- [ ] **Step 5: Verify no oc binary in built image**

Run: `podman run --rm --entrypoint /bin/sh oadp-must-gather-test:latest -c "which oc 2>/dev/null || echo 'oc not found (expected)'"`
Expected: `oc not found (expected)`
