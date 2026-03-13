package helm

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

// makeCertProviderBundle returns a bundle with a validating webhook so that
// cert-manager resources are generated.
func makeCertProviderBundle() *bundle.RegistryV1 {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"
	return makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ValidatingAdmissionWebhook,
				GenerateName:            "validate-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		}
	})
}

// renderCertProvider generates a helm chart from the bundle, renders the
// cert-manager.yaml template with the given values, and parses the result
// into a slice of unstructured.Unstructured objects.
func renderCertProvider(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) []unstructured.Unstructured {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var certYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "cert-manager.yaml") {
			certYAML = data
			break
		}
	}
	if certYAML == "" {
		return nil
	}

	var objects []unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(certYAML), 4096)
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			break
		}
		if obj.GetKind() != "" {
			objects = append(objects, obj)
		}
	}
	return objects
}

func TestCertProvider_WithWebhooks(t *testing.T) {
	b := makeCertProviderBundle()

	objects := renderCertProvider(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})

	require.NotEmpty(t, objects, "expected cert-manager objects to be rendered")

	// We expect an Issuer and a Certificate
	var issuer, certificate *unstructured.Unstructured
	for i := range objects {
		switch objects[i].GetKind() {
		case "Issuer":
			issuer = &objects[i]
		case "Certificate":
			certificate = &objects[i]
		}
	}

	require.NotNil(t, issuer, "expected an Issuer object")
	require.NotNil(t, certificate, "expected a Certificate object")

	// Issuer assertions
	assert.Equal(t, "cert-manager.io/v1", issuer.GetAPIVersion())
	assert.Equal(t, "test-ns", issuer.GetNamespace())

	// Certificate assertions
	assert.Equal(t, "cert-manager.io/v1", certificate.GetAPIVersion())
	assert.Equal(t, "test-ns", certificate.GetNamespace())

	// secretName should match the cert name
	expectedCertName := certNameForDeployment("controller-manager")
	assert.Equal(t, expectedCertName, certificate.GetName())

	secretName, found, err := unstructured.NestedString(certificate.Object, "spec", "secretName")
	require.NoError(t, err)
	require.True(t, found, "spec.secretName should be set")
	assert.Equal(t, expectedCertName, secretName)

	// dnsNames should reference the service name
	expectedSvcName := serviceNameForDeployment("controller-manager")
	dnsNames, found, err := unstructured.NestedStringSlice(certificate.Object, "spec", "dnsNames")
	require.NoError(t, err)
	require.True(t, found, "spec.dnsNames should be set")
	require.Len(t, dnsNames, 3)
	assert.Equal(t, expectedSvcName+".test-ns", dnsNames[0])
	assert.Equal(t, expectedSvcName+".test-ns.svc", dnsNames[1])
	assert.Equal(t, expectedSvcName+".test-ns.svc.cluster.local", dnsNames[2])
}

func TestCertProvider_NoWebhooks(t *testing.T) {
	b := makeMinimalBundle()

	c, err := Generate(b)
	require.NoError(t, err)

	// Verify no cert-manager template exists when there are no webhooks
	for _, tmpl := range c.Templates {
		assert.False(t, strings.HasSuffix(tmpl.Name, "cert-manager.yaml"),
			"cert-manager.yaml should not be generated when there are no webhooks")
	}
}

func TestCertProvider_CertProviderDisabled(t *testing.T) {
	b := makeCertProviderBundle()

	objects := renderCertProvider(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "",
	})

	assert.Empty(t, objects, "no cert-manager objects should be rendered when certProvider is empty")
}

func TestCertProvider_IssuerName(t *testing.T) {
	b := makeCertProviderBundle()

	objects := renderCertProvider(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})

	require.NotEmpty(t, objects)

	var issuer *unstructured.Unstructured
	var certificate *unstructured.Unstructured
	for i := range objects {
		switch objects[i].GetKind() {
		case "Issuer":
			issuer = &objects[i]
		case "Certificate":
			certificate = &objects[i]
		}
	}

	require.NotNil(t, issuer)
	require.NotNil(t, certificate)

	// The certificate's issuerRef.name should match the issuer's name
	issuerRefName, found, err := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
	require.NoError(t, err)
	require.True(t, found, "spec.issuerRef.name should be set")
	assert.Equal(t, issuer.GetName(), issuerRefName)
}
