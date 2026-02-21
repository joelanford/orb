package certproviders

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joelanford/orb/internal/convert"
)

func TestOpenshiftServiceCaCertificateProvider_InjectCABundle(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName:  "test-cert",
		Namespace: "test-ns",
	}
	p := OpenshiftServiceCaCertificateProvider{}

	t.Run("ValidatingWebhookConfiguration", func(t *testing.T) {
		obj := &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "true", obj.GetAnnotations()[openshiftServiceCAInjectCABundleAnnotation])
	})

	t.Run("MutatingWebhookConfiguration", func(t *testing.T) {
		obj := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "true", obj.GetAnnotations()[openshiftServiceCAInjectCABundleAnnotation])
	})

	t.Run("CustomResourceDefinition", func(t *testing.T) {
		obj := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "test-crd"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "true", obj.GetAnnotations()[openshiftServiceCAInjectCABundleAnnotation])
	})

	t.Run("Service", func(t *testing.T) {
		obj := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "test-svc"},
		}
		err := p.InjectCABundle(obj, cfg)
		require.NoError(t, err)
		assert.Equal(t, "test-cert", obj.GetAnnotations()[openshiftServiceCAServingCertNameAnnotation])
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

func TestOpenshiftServiceCaCertificateProvider_AdditionalObjects(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName:  "test-cert",
		Namespace: "test-ns",
	}
	p := OpenshiftServiceCaCertificateProvider{}
	objs, err := p.AdditionalObjects(cfg)
	require.NoError(t, err)
	assert.Nil(t, objs)
}

func TestOpenshiftServiceCaCertificateProvider_GetCertSecretInfo(t *testing.T) {
	cfg := convert.CertificateProvisionerConfig{
		CertName: "my-cert",
	}
	p := OpenshiftServiceCaCertificateProvider{}
	info := p.GetCertSecretInfo(cfg)

	assert.Equal(t, "my-cert", info.SecretName)
	assert.Equal(t, "tls.crt", info.CertificateKey)
	assert.Equal(t, "tls.key", info.PrivateKeyKey)
}
