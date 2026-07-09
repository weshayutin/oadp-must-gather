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
