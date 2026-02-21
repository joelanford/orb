package convert

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockCertProvider struct {
	injectCABundleErr    error
	additionalObjects    []unstructured.Unstructured
	additionalObjectsErr error
	certSecretInfo       CertSecretInfo
}

func (m *mockCertProvider) InjectCABundle(_ client.Object, _ CertificateProvisionerConfig) error {
	return m.injectCABundleErr
}

func (m *mockCertProvider) AdditionalObjects(_ CertificateProvisionerConfig) ([]unstructured.Unstructured, error) {
	return m.additionalObjects, m.additionalObjectsErr
}

func (m *mockCertProvider) GetCertSecretInfo(_ CertificateProvisionerConfig) CertSecretInfo {
	return m.certSecretInfo
}

func TestCertificateProvisioner_InjectCABundle(t *testing.T) {
	t.Run("nil provider does nothing", func(t *testing.T) {
		cp := CertificateProvisioner{}
		err := cp.InjectCABundle(nil)
		assert.NoError(t, err)
	})

	t.Run("with provider delegates", func(t *testing.T) {
		expectedErr := errors.New("inject error")
		mock := &mockCertProvider{injectCABundleErr: expectedErr}
		cp := CertificateProvisioner{CertProvider: mock}
		err := cp.InjectCABundle(nil)
		assert.Equal(t, expectedErr, err)
	})
}

func TestCertificateProvisioner_AdditionalObjects(t *testing.T) {
	t.Run("nil provider returns nil", func(t *testing.T) {
		cp := CertificateProvisioner{}
		objs, err := cp.AdditionalObjects()
		assert.NoError(t, err)
		assert.Nil(t, objs)
	})

	t.Run("with provider returns objects", func(t *testing.T) {
		expected := []unstructured.Unstructured{
			{Object: map[string]interface{}{"kind": "Certificate"}},
		}
		mock := &mockCertProvider{additionalObjects: expected}
		cp := CertificateProvisioner{CertProvider: mock}
		objs, err := cp.AdditionalObjects()
		assert.NoError(t, err)
		assert.Equal(t, expected, objs)
	})

	t.Run("with provider returns error", func(t *testing.T) {
		expectedErr := errors.New("additional objects error")
		mock := &mockCertProvider{additionalObjectsErr: expectedErr}
		cp := CertificateProvisioner{CertProvider: mock}
		objs, err := cp.AdditionalObjects()
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, objs)
	})
}

func TestCertificateProvisioner_GetCertSecretInfo(t *testing.T) {
	t.Run("nil provider returns nil", func(t *testing.T) {
		cp := CertificateProvisioner{}
		info := cp.GetCertSecretInfo()
		assert.Nil(t, info)
	})

	t.Run("with provider returns info", func(t *testing.T) {
		expected := CertSecretInfo{
			SecretName:     "my-secret",
			CertificateKey: "tls.crt",
			PrivateKeyKey:  "tls.key",
		}
		mock := &mockCertProvider{certSecretInfo: expected}
		cp := CertificateProvisioner{CertProvider: mock}
		info := cp.GetCertSecretInfo()
		require.NotNil(t, info)
		assert.Equal(t, expected, *info)
	})
}

func TestCertProvisionerFor(t *testing.T) {
	t.Run("constructs service and cert names", func(t *testing.T) {
		mock := &mockCertProvider{}
		opts := Options{
			InstallNamespace:    "test-ns",
			CertificateProvider: mock,
		}
		cp := CertProvisionerFor("my-controller", opts)
		assert.Equal(t, "my-controller-service", cp.ServiceName)
		assert.Equal(t, "my-controller-service-cert", cp.CertName)
		assert.Equal(t, "test-ns", cp.Namespace)
		assert.Equal(t, mock, cp.CertProvider)
	})

	t.Run("dots replaced with hyphens", func(t *testing.T) {
		opts := Options{
			InstallNamespace:    "test-ns",
			CertificateProvider: &mockCertProvider{},
		}
		cp := CertProvisionerFor("my.dotted.name", opts)
		assert.Equal(t, "my-dotted-name-service", cp.ServiceName)
		assert.Equal(t, "my-dotted-name-service-cert", cp.CertName)
	})

	t.Run("nil certificate provider", func(t *testing.T) {
		opts := Options{
			InstallNamespace: "test-ns",
		}
		cp := CertProvisionerFor("my-controller", opts)
		assert.Nil(t, cp.CertProvider)
		assert.Equal(t, "my-controller-service", cp.ServiceName)
	})
}
