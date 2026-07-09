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
