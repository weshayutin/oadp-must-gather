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
			"password":       []byte("super-secret-password"),
			"tls.crt":        []byte("-----BEGIN CERTIFICATE-----"),
			"ca.crt":         []byte("-----BEGIN CERTIFICATE-----"),
			"service-ca.crt": []byte("-----BEGIN CERTIFICATE-----"),
			"api-key":        []byte("abc123"),
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
