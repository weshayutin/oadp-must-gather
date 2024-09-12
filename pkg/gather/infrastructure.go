package gather

import (
	"context"
	"fmt"

	openshiftconfigv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO this and GatherClusterVersion can be modified to be a single function
func Infrastructure(clusterClient client.Client) (*openshiftconfigv1.Infrastructure, error) {
	infrastructureList := &openshiftconfigv1.InfrastructureList{}
	err := clusterClient.List(context.Background(), infrastructureList)
	if err != nil {
		return nil, err
	}
	if len(infrastructureList.Items) == 0 {
		return nil, fmt.Errorf("no Infrastructure found in cluster")
	}
	return &infrastructureList.Items[0], nil
}
