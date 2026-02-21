package certproviders

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joelanford/orb/internal/convert"
)

func TestCertManagerCertificateProvider_InjectCABundle(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName:  "test-cert",
		Namespace: "test-ns",
	}
	p := CertManagerCertificateProvider{}

	t.Run("ValidatingWebhookConfiguration", func(t *testing.T) {
		obj := &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "test-ns/test-cert", obj.GetAnnotations()[certManagerInjectCAAnnotation])
	})

	t.Run("MutatingWebhookConfiguration", func(t *testing.T) {
		obj := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "test-ns/test-cert", obj.GetAnnotations()[certManagerInjectCAAnnotation])
	})

	t.Run("CustomResourceDefinition", func(t *testing.T) {
		obj := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "test-crd"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "test-ns/test-cert", obj.GetAnnotations()[certManagerInjectCAAnnotation])
	})

	t.Run("UnsupportedType", func(t *testing.T) {
		obj := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-deploy"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Empty(t, obj.GetAnnotations())
	})
}

func TestCertManagerCertificateProvider_GetCertSecretInfo(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName: "my-cert",
	}
	p := CertManagerCertificateProvider{}
	info := p.GetCertSecretInfo(cfg)

	assert.Equal(t, "my-cert", info.SecretName)
	assert.Equal(t, "tls.crt", info.CertificateKey)
	assert.Equal(t, "tls.key", info.PrivateKeyKey)
}

func TestCertManagerCertificateProvider_AdditionalObjects(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName:    "my-cert",
		Namespace:   "my-ns",
		ServiceName: "my-svc",
	}
	p := CertManagerCertificateProvider{}
	objs, err := p.AdditionalObjects(cfg)
	require.NoError(t, err)
	require.Len(t, objs, 2)

	// First object should be the Issuer
	issuer := objs[0]
	assert.Equal(t, "Issuer", issuer.GetKind())
	assert.Equal(t, "my-ns", issuer.GetNamespace())
	assert.Equal(t, convert.ObjectNameForBaseAndSuffix("my-cert", "selfsigned-issuer"), issuer.GetName())

	// Second object should be the Certificate
	cert := objs[1]
	assert.Equal(t, "Certificate", cert.GetKind())
	assert.Equal(t, "my-ns", cert.GetNamespace())
	assert.Equal(t, "my-cert", cert.GetName())
}
