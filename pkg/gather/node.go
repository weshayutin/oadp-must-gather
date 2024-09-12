package gather

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO duplication?
func ALLNodes(clusterClient client.Client) (*corev1.NodeList, error) {
	nodeList := &corev1.NodeList{}
	err := clusterClient.List(context.Background(), nodeList)
	if err != nil {
		return nil, err
	}
	if len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("no Node found in cluster")
	}
	return nodeList, nil
}
