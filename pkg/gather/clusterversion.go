package gather

import (
	"context"
	"fmt"

	openshiftconfigv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ClusterVersion(clusterClient client.Client) (*openshiftconfigv1.ClusterVersion, error) {
	clusterVersionList := &openshiftconfigv1.ClusterVersionList{}
	err := clusterClient.List(context.Background(), clusterVersionList)
	if err != nil {
		return nil, err
	}
	if len(clusterVersionList.Items) == 0 {
		return nil, fmt.Errorf("no ClusterVersion found in cluster")
	}
	return &clusterVersionList.Items[0], nil
}
