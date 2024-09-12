package gather

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// subscriptionList := &operatorsv1alpha1.SubscriptionList{}
// err = clusterClient.List(context.Background(), subscriptionList)
// if err != nil {
// 	fmt.Println(err)
// }
// for _, sub := range subscriptionList.Items {
// 	// prod? "redhat-oadp-operator"
// 	// other packages that should be important for us?
// 	// dev? "oadp-operator" https://github.com/openshift/oadp-operator/blob/5601dcfd0a07468f496ddb70ab570ccff1b4f0cc/bundle/metadata/annotations.yaml#L6
// 	fmt.Printf("Found '%v' operator version '%v' installed in '%v' namespace\n", sub.Spec.Package, sub.Spec.StartingCSV, sub.Namespace)
// }

// TODO CSV and/or Subscription???
func AllClusterServiceVersions(clusterClient client.Client) (*operatorsv1alpha1.ClusterServiceVersionList, error) {
	clusterServiceVersionList := &operatorsv1alpha1.ClusterServiceVersionList{}
	err := clusterClient.List(context.Background(), clusterServiceVersionList)
	if err != nil {
		return nil, err
	}
	if len(clusterServiceVersionList.Items) == 0 {
		return nil, fmt.Errorf("no ClusterServiceVersion found in cluster")
	}
	return clusterServiceVersionList, nil
}
