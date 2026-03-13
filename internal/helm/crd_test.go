package helm

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

// makeCRDBundle returns a minimal bundle with a CRD and optional conversion webhook.
func makeCRDBundle(crd apiextensionsv1.CustomResourceDefinition, withConversionWebhook bool) *bundle.RegistryV1 {
	return makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CRDs = []apiextensionsv1.CustomResourceDefinition{crd}
		b.CSV.Spec.CustomResourceDefinitions.Owned = []v1alpha1.CRDDescription{
			{Name: crd.Name, Version: crd.Spec.Versions[0].Name, Kind: crd.Spec.Names.Kind},
		}
		if withConversionWebhook {
			path := "/convert"
			b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
				{
					Type:                    v1alpha1.ConversionWebhook,
					GenerateName:            "convert-test",
					DeploymentName:          "controller-manager",
					ContainerPort:           443,
					WebhookPath:             &path,
					AdmissionReviewVersions: []string{"v1"},
					ConversionCRDs:          []string{crd.Name},
				},
			}
		}
	})
}

// renderCRDs generates a helm chart from the bundle, renders the crd.yaml
// template with the given values, and parses the result into a slice of
// CustomResourceDefinition structs.
func renderCRDs(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) []apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var crdYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "crd.yaml") {
			crdYAML = data
			break
		}
	}
	require.NotEmpty(t, crdYAML, "crd.yaml not found in rendered output")

	var crds []apiextensionsv1.CustomResourceDefinition
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(crdYAML), 4096)
	for {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			break
		}
		if crd.Name != "" {
			crds = append(crds, crd)
		}
	}
	require.NotEmpty(t, crds, "failed to parse any CRD from rendered YAML:\n%s", crdYAML)
	return crds
}

func simpleCRD() apiextensionsv1.CustomResourceDefinition {
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tests.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "Test",
				Plural: "tests",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}
}

func multiVersionCRD() apiextensionsv1.CustomResourceDefinition {
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tests.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "Test",
				Plural: "tests",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1beta1", Served: true, Storage: false},
			},
		},
	}
}

// --- Simple CRD (no conversion webhook) ---

func TestCRD_Simple(t *testing.T) {
	b := makeCRDBundle(simpleCRD(), false)

	crds := renderCRDs(t, b, map[string]any{"watchNamespace": ""})
	require.Len(t, crds, 1)

	crd := crds[0]
	assert.Equal(t, "tests.example.com", crd.Name)
	assert.Equal(t, "example.com", crd.Spec.Group)
	assert.Equal(t, "Test", crd.Spec.Names.Kind)
	assert.Equal(t, "tests", crd.Spec.Names.Plural)
	require.Len(t, crd.Spec.Versions, 1)
	assert.Equal(t, "v1", crd.Spec.Versions[0].Name)
	assert.True(t, crd.Spec.Versions[0].Served)
	assert.True(t, crd.Spec.Versions[0].Storage)

	// No conversion webhook should be configured
	assert.Nil(t, crd.Spec.Conversion)
}

// --- CRD with conversion webhook ---

func TestCRD_WithConversionWebhook_CertManager(t *testing.T) {
	b := makeCRDBundle(multiVersionCRD(), true)

	crds := renderCRDs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	require.Len(t, crds, 1)

	crd := crds[0]
	assert.Equal(t, "tests.example.com", crd.Name)
	assert.Equal(t, "example.com", crd.Spec.Group)
	require.Len(t, crd.Spec.Versions, 2)
	assert.Equal(t, "v1", crd.Spec.Versions[0].Name)
	assert.Equal(t, "v1beta1", crd.Spec.Versions[1].Name)

	// Conversion webhook should be configured
	require.NotNil(t, crd.Spec.Conversion)
	assert.Equal(t, apiextensionsv1.WebhookConverter, crd.Spec.Conversion.Strategy)
	require.NotNil(t, crd.Spec.Conversion.Webhook)
	require.NotNil(t, crd.Spec.Conversion.Webhook.ClientConfig)
	require.NotNil(t, crd.Spec.Conversion.Webhook.ClientConfig.Service)
	assert.Equal(t, "test-ns", crd.Spec.Conversion.Webhook.ClientConfig.Service.Namespace)
	assert.Equal(t, "/convert", *crd.Spec.Conversion.Webhook.ClientConfig.Service.Path)
	assert.Equal(t, []string{"v1"}, crd.Spec.Conversion.Webhook.ConversionReviewVersions)

	// cert-manager annotation should be present
	require.NotNil(t, crd.Annotations)
	certName := certNameForDeployment("controller-manager")
	assert.Equal(t, "test-ns/"+certName, crd.Annotations["cert-manager.io/inject-ca-from"])
}

func TestCRD_WithConversionWebhook_ServiceCA(t *testing.T) {
	b := makeCRDBundle(multiVersionCRD(), true)

	crds := renderCRDs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "service-ca",
	})
	require.Len(t, crds, 1)

	crd := crds[0]
	// Conversion webhook should still be configured
	require.NotNil(t, crd.Spec.Conversion)
	assert.Equal(t, apiextensionsv1.WebhookConverter, crd.Spec.Conversion.Strategy)

	// service-ca annotation should be present
	require.NotNil(t, crd.Annotations)
	assert.Equal(t, "true", crd.Annotations["service.beta.openshift.io/inject-cabundle"])
}

func TestCRD_WithConversionWebhook_NoCertProvider(t *testing.T) {
	b := makeCRDBundle(multiVersionCRD(), true)

	crds := renderCRDs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "",
	})
	require.Len(t, crds, 1)

	crd := crds[0]
	// Conversion webhook should still be configured
	require.NotNil(t, crd.Spec.Conversion)
	assert.Equal(t, apiextensionsv1.WebhookConverter, crd.Spec.Conversion.Strategy)

	// No cert annotations should be present
	if crd.Annotations != nil {
		assert.Empty(t, crd.Annotations["cert-manager.io/inject-ca-from"])
		assert.Empty(t, crd.Annotations["service.beta.openshift.io/inject-cabundle"])
	}
}

// --- Simple CRD has no cert annotations regardless of certProvider ---

func TestCRD_Simple_NoCertAnnotations(t *testing.T) {
	b := makeCRDBundle(simpleCRD(), false)

	crds := renderCRDs(t, b, map[string]any{"watchNamespace": ""})
	require.Len(t, crds, 1)

	crd := crds[0]
	if crd.Annotations != nil {
		assert.Empty(t, crd.Annotations["cert-manager.io/inject-ca-from"])
		assert.Empty(t, crd.Annotations["service.beta.openshift.io/inject-cabundle"])
	}
}

// ---- Unit tests for CRD utility functions ----

func TestEscapeHelmExceptDirectives(t *testing.T) {
	t.Run("PreservesReleaseNamespace", func(t *testing.T) {
		input := "namespace: {{ .Release.Namespace }}"
		assert.Equal(t, input, escapeHelmExceptDirectives(input))
	})

	t.Run("PreservesValues", func(t *testing.T) {
		input := "value: {{ .Values.foo }}"
		assert.Equal(t, input, escapeHelmExceptDirectives(input))
	})

	t.Run("PreservesConditionals", func(t *testing.T) {
		input := "{{- if .Values.something }}"
		assert.Equal(t, input, escapeHelmExceptDirectives(input))
	})

	t.Run("EscapesLiteralBraces", func(t *testing.T) {
		input := "annotation: {{some-literal}}"
		result := escapeHelmExceptDirectives(input)
		assert.Contains(t, result, "{{ `{{` }}")
	})
}

func TestInjectMetadataAnnotation(t *testing.T) {
	t.Run("WithExistingAnnotations", func(t *testing.T) {
		yamlStr := `apiVersion: v1
kind: Test
metadata:
  name: test
  annotations:
    existing: value
spec:
  foo: bar
`
		result := injectMetadataAnnotation(yamlStr, "new-annotation: new-value")
		assert.Contains(t, result, "existing: value")
		assert.Contains(t, result, "new-annotation: new-value")
	})

	t.Run("WithoutAnnotations", func(t *testing.T) {
		yamlStr := `apiVersion: v1
kind: Test
metadata:
  name: test
spec:
  foo: bar
`
		result := injectMetadataAnnotation(yamlStr, "new-annotation: new-value")
		assert.Contains(t, result, "annotations:")
		assert.Contains(t, result, "new-annotation: new-value")
	})
}

func TestInsertCRDCertAnnotations(t *testing.T) {
	crdYAML := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tests.example.com
spec:
  group: example.com
`
	result := insertCRDCertAnnotations(crdYAML, "my-cert")
	assert.Contains(t, result, "cert-manager")
	assert.Contains(t, result, "service-ca")
	assert.Contains(t, result, "my-cert")
	assert.Contains(t, result, "{{- else }}")
}
