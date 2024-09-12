package gather

import (
	"context"
	"fmt"

	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AllStorageClasses(clusterClient client.Client) (*storagev1.StorageClassList, error) {
	storageClassList := &storagev1.StorageClassList{}
	err := clusterClient.List(context.Background(), storageClassList)
	if err != nil {
		return nil, err
	}
	if len(storageClassList.Items) == 0 {
		return nil, fmt.Errorf("no StorageClass found in cluster")
	}
	return storageClassList, nil
}
