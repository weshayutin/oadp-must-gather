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
	secretList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("SecretList"))
	for _, item := range list.Items {
		secret := &corev1.Secret{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, secret); err != nil {
			return nil, err
		}
		secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
		secretList.Items = append(secretList.Items, *secret)
	}
	return secretList, nil
}
