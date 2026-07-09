package inspect

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func allGVRListKinds() map[schema.GroupVersionResource]string {
	m := make(map[schema.GroupVersionResource]string)
	for _, res := range namespacedResourcesToCollect() {
		kind := strings.TrimSuffix(res.gvr.Resource, "s")
		kind = strings.ToUpper(kind[:1]) + kind[1:]
		m[res.gvr] = kind + "List"
	}
	return m
}

func newFakeDynamicClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGVRListKinds())
}

func TestGatherNamespaceData_SkipsMissingResource(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	kubeClient := kubefake.NewSimpleClientset(ns)
	dynamicClient := newFakeDynamicClient()

	dynamicClient.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		la := action.(clienttesting.ListAction)
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: la.GetResource().Group, Resource: la.GetResource().Resource},
			"",
		)
	})

	dir := t.TempDir()
	err := gatherNamespaceData(context.Background(), kubeClient, dynamicClient, dir, "test-ns")
	if err != nil {
		t.Errorf("expected no error when all resources are NotFound, got: %v", err)
	}
}

func TestGatherNamespaceData_PropagatesForbiddenError(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	kubeClient := kubefake.NewSimpleClientset(ns)
	dynamicClient := newFakeDynamicClient()

	dynamicClient.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		la := action.(clienttesting.ListAction)
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: la.GetResource().Resource},
			"",
			nil,
		)
	})

	dir := t.TempDir()
	err := gatherNamespaceData(context.Background(), kubeClient, dynamicClient, dir, "test-ns")
	if err == nil {
		t.Fatal("expected aggregate error for Forbidden responses, got nil")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected error to mention 'forbidden', got: %v", err)
	}
}

func TestGatherNamespaceData_MixedErrors(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	kubeClient := kubefake.NewSimpleClientset(ns)
	dynamicClient := newFakeDynamicClient()

	dynamicClient.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		la := action.(clienttesting.ListAction)
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: la.GetResource().Group, Resource: la.GetResource().Resource},
			"",
		)
	})
	dynamicClient.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "pods"},
			"",
			nil,
		)
	})

	dir := t.TempDir()
	err := gatherNamespaceData(context.Background(), kubeClient, dynamicClient, dir, "test-ns")
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "pods") {
		t.Errorf("expected aggregate to include pods Forbidden error, got: %v", err)
	}
	if strings.Contains(errStr, "egressfirewalls") {
		t.Errorf("expected aggregate to NOT include skipped egressfirewalls NotFound, got: %v", err)
	}
}
