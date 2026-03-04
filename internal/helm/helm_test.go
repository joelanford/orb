package helm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	bsemver "github.com/blang/semver/v4"
	opversion "github.com/operator-framework/api/pkg/lib/version"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/joelanford/orb/internal/bundle"
)

// makeMinimalBundle returns a minimal valid *bundle.RegistryV1.
func makeMinimalBundle(opts ...func(*bundle.RegistryV1)) *bundle.RegistryV1 {
	b := &bundle.RegistryV1{
		PackageName: "test-operator",
		CSV: v1alpha1.ClusterServiceVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-operator.v1.0.0",
			},
			Spec: v1alpha1.ClusterServiceVersionSpec{
				Version:     semverVersion("1.0.0"),
				DisplayName: "Test Operator",
				InstallModes: []v1alpha1.InstallMode{
					{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
				},
				InstallStrategy: v1alpha1.NamedInstallStrategy{
					StrategySpec: v1alpha1.StrategyDetailsDeployment{
						DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
							{
								Name: "controller-manager",
								Spec: appsv1.DeploymentSpec{
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"app": "test"},
									},
									Template: corev1.PodTemplateSpec{
										ObjectMeta: metav1.ObjectMeta{
											Labels: map[string]string{"app": "test"},
										},
										Spec: corev1.PodSpec{
											Containers: []corev1.Container{
												{
													Name:  "manager",
													Image: "registry.io/test-operator:v1.0.0",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func semverVersion(v string) opversion.OperatorVersion {
	return opversion.OperatorVersion{Version: bsemver.MustParse(v)}
}

// ---- Tests for the main Generate function ----

func TestGenerate_MinimalBundle(t *testing.T) {
	b := makeMinimalBundle()
	c, err := Generate(b)
	require.NoError(t, err)

	// Metadata
	assert.Equal(t, "test-operator", c.Metadata.Name)
	assert.Equal(t, "v2", c.Metadata.APIVersion)
	assert.Equal(t, "1.0.0", c.Metadata.Version)
	assert.Equal(t, "application", c.Metadata.Type)

	// Values
	assert.NotNil(t, c.Values)
	assert.Contains(t, c.Values, "watchNamespace")
	assert.NotContains(t, c.Values, "certProvider")

	// Schema
	assert.NotEmpty(t, c.Schema)

	// Required templates exist
	templateNames := chartTemplateNames(c)
	assert.Contains(t, templateNames, "templates/_helpers.tpl")
	assert.Contains(t, templateNames, "templates/deployment.yaml")

	// No webhook/service/cert-manager templates
	assert.NotContains(t, templateNames, "templates/webhook.yaml")
	assert.NotContains(t, templateNames, "templates/service.yaml")
	assert.NotContains(t, templateNames, "templates/cert-manager.yaml")
}

func TestGenerate_WithCRDs(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CRDs = []apiextensionsv1.CustomResourceDefinition{
			{
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
			},
		}
		b.CSV.Spec.CustomResourceDefinitions.Owned = []v1alpha1.CRDDescription{
			{Name: "tests.example.com", Version: "v1", Kind: "Test"},
		}
	})
	c, err := Generate(b)
	require.NoError(t, err)
	assert.Contains(t, chartTemplateNames(c), "templates/crd.yaml")
}

func TestGenerate_WithWebhooks(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"

	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
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
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"example.com"},
							APIVersions: []string{"v1"},
							Resources:   []string{"tests"},
						},
					},
				},
			},
		}
	})

	c, err := Generate(b)
	require.NoError(t, err)

	templateNames := chartTemplateNames(c)
	assert.Contains(t, templateNames, "templates/webhook.yaml")
	assert.Contains(t, templateNames, "templates/service.yaml")
	assert.Contains(t, templateNames, "templates/cert-manager.yaml")

	// Values should contain certProvider
	assert.Contains(t, c.Values, "certProvider")
}

func TestGenerate_WithConversionWebhook(t *testing.T) {
	path := "/convert"

	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
		}
		b.CRDs = []apiextensionsv1.CustomResourceDefinition{
			{
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
			},
		}
		b.CSV.Spec.CustomResourceDefinitions.Owned = []v1alpha1.CRDDescription{
			{Name: "tests.example.com", Version: "v1", Kind: "Test"},
		}
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ConversionWebhook,
				GenerateName:            "convert-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				AdmissionReviewVersions: []string{"v1"},
				ConversionCRDs:          []string{"tests.example.com"},
			},
		}
	})

	c, err := Generate(b)
	require.NoError(t, err)

	crdYAML := string(chartTemplateData(c, "templates/crd.yaml"))
	assert.Contains(t, crdYAML, "cert-manager")
	assert.Contains(t, crdYAML, "certProvider")
}

func TestGenerate_WithAdditionalResources(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.Others = []unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
		}
	})

	c, err := Generate(b)
	require.NoError(t, err)
	assert.Contains(t, chartTemplateNames(c), "templates/additional.yaml")
}

func TestGenerate_WithPermissions(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"secrets"}},
				},
			},
		}
	})

	c, err := Generate(b)
	require.NoError(t, err)
	assert.Contains(t, chartTemplateNames(c), "templates/clusterrole.yaml")
}

func TestGenerate_ValidationFailure(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.PackageName = ""
	})

	_, err := Generate(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundle validation failed")
}

// chartTemplateNames returns template names from a chart.
func chartTemplateNames(c *chart.Chart) []string {
	var names []string
	for _, t := range c.Templates {
		names = append(names, t.Name)
	}
	return names
}

// chartTemplateData returns template data for a given name, or nil if not found.
func chartTemplateData(c *chart.Chart, name string) []byte {
	for _, t := range c.Templates {
		if t.Name == name {
			return t.Data
		}
	}
	return nil
}

// ---- Tests for sub-generators ----

func TestGenerateServiceAccounts(t *testing.T) {
	t.Run("NamedSA", func(t *testing.T) {
		b := makeMinimalBundle(func(b *bundle.RegistryV1) {
			b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
				{ServiceAccountName: "my-sa", Rules: []rbacv1.PolicyRule{}},
			}
		})
		data, err := generateServiceAccounts(b)
		require.NoError(t, err)
		assert.Contains(t, string(data), "name: my-sa")
	})

	t.Run("DefaultSkipped", func(t *testing.T) {
		b := makeMinimalBundle(func(b *bundle.RegistryV1) {
			b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
				{ServiceAccountName: "", Rules: []rbacv1.PolicyRule{}}, // empty => "default"
			}
		})
		data, err := generateServiceAccounts(b)
		require.NoError(t, err)
		// "default" SA should be skipped
		assert.Empty(t, data)
	})

	t.Run("MultipleSorted", func(t *testing.T) {
		b := makeMinimalBundle(func(b *bundle.RegistryV1) {
			b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
				{ServiceAccountName: "bravo", Rules: []rbacv1.PolicyRule{}},
				{ServiceAccountName: "alpha", Rules: []rbacv1.PolicyRule{}},
			}
		})
		data, err := generateServiceAccounts(b)
		require.NoError(t, err)
		s := string(data)
		alphaIdx := strings.Index(s, "name: alpha")
		bravoIdx := strings.Index(s, "name: bravo")
		assert.Greater(t, bravoIdx, alphaIdx, "alpha should come before bravo")
	})
}

func TestGenerateRBAC_ClusterPermissions(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"secrets"}},
				},
			},
		}
	})

	data, err := generateRBAC(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "kind: ClusterRole")
	assert.Contains(t, s, "kind: ClusterRoleBinding")
	assert.Contains(t, s, "controller-manager")
}

func TestGenerateRBAC_Permissions(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "my-sa",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
				},
			},
		}
	})

	data, err := generateRBAC(b)
	require.NoError(t, err)
	s := string(data)
	// AllNamespaces block
	assert.Contains(t, s, "{{- if eq .Values.watchNamespace \"\" }}")
	assert.Contains(t, s, "kind: ClusterRole")
	// Non-AllNamespaces block
	assert.Contains(t, s, "{{- else }}")
	assert.Contains(t, s, "kind: Role")
	assert.Contains(t, s, "kind: RoleBinding")
}

func TestGenerateDeployments(t *testing.T) {
	t.Run("SingleDeployment", func(t *testing.T) {
		b := makeMinimalBundle()
		data, err := generateDeployments(b, sets.New[string]())
		require.NoError(t, err)
		s := string(data)
		assert.Contains(t, s, "kind: Deployment")
		assert.Contains(t, s, "name: controller-manager")
		assert.Contains(t, s, "{{ .Release.Namespace }}")
	})

	t.Run("WebhookDeploymentHasCertVolumeConditionals", func(t *testing.T) {
		b := makeMinimalBundle()
		data, err := generateDeployments(b, sets.New[string]("controller-manager"))
		require.NoError(t, err)
		s := string(data)
		assert.Contains(t, s, "webhook-cert")
		assert.Contains(t, s, "{{- if ne .Values.certProvider \"\" }}")
	})
}

func TestGenerateDeployments_AnnotationValuesSurviveYAMLRoundTrip(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Annotations = map[string]string{
			"certified": "false",
			"count":     "3",
			"nullable":  "null",
			"plain":     "hello",
		}
	})
	data, err := generateDeployments(b, sets.New[string]())
	require.NoError(t, err)
	s := string(data)

	// Boolean-like, numeric, and null-like values must be quoted so YAML
	// does not coerce them away from strings.
	assert.Contains(t, s, `certified: "false"`)
	assert.Contains(t, s, `count: "3"`)
	assert.Contains(t, s, `nullable: "null"`)

	// Plain strings stay unquoted.
	assert.Contains(t, s, "plain: hello")
}

func TestGenerateCRDs_Simple(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CRDs = []apiextensionsv1.CustomResourceDefinition{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "tests.example.com"},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "example.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Test", Plural: "tests"},
					Scope: apiextensionsv1.NamespaceScoped,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{Name: "v1", Served: true, Storage: true},
					},
				},
			},
		}
	})

	data, err := generateCRDs(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "tests.example.com")
	// No conversion webhook references
	assert.NotContains(t, s, "certProvider")
}

func TestGenerateCRDs_WithConversionWebhook(t *testing.T) {
	path := "/convert"
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CRDs = []apiextensionsv1.CustomResourceDefinition{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "tests.example.com"},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "example.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Test", Plural: "tests"},
					Scope: apiextensionsv1.NamespaceScoped,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{Name: "v1", Served: true, Storage: true},
					},
				},
			},
		}
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ConversionWebhook,
				GenerateName:            "convert-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				AdmissionReviewVersions: []string{"v1"},
				ConversionCRDs:          []string{"tests.example.com"},
			},
		}
	})

	data, err := generateCRDs(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "cert-manager")
	assert.Contains(t, s, "service-ca")
	assert.Contains(t, s, "certProvider")
	assert.Contains(t, s, "Webhook")
}

func TestGenerateWebhooks_Validating(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"

	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
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

	data, err := generateWebhooks(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "kind: ValidatingWebhookConfiguration")
	assert.Contains(t, s, "validate-test")
}

func TestGenerateWebhooks_Mutating(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/mutate"
	reinvocationPolicy := admissionregistrationv1.IfNeededReinvocationPolicy

	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.MutatingAdmissionWebhook,
				GenerateName:            "mutate-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				ReinvocationPolicy:      &reinvocationPolicy,
				AdmissionReviewVersions: []string{"v1"},
			},
		}
	})

	data, err := generateWebhooks(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "kind: MutatingWebhookConfiguration")
	assert.Contains(t, s, "mutate-test")
	assert.Contains(t, s, "reinvocationPolicy")
}

func TestGenerateWebhooks_SkipsConversion(t *testing.T) {
	path := "/convert"

	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ConversionWebhook,
				GenerateName:            "convert-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				AdmissionReviewVersions: []string{"v1"},
				ConversionCRDs:          []string{"tests.example.com"},
			},
		}
	})

	data, err := generateWebhooks(b)
	require.NoError(t, err)
	// Conversion webhooks should not emit a webhook configuration
	assert.Empty(t, data)
}

func TestGenerateWebhookServices(t *testing.T) {
	path := "/validate"
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ValidatingAdmissionWebhook,
				GenerateName:            "validate-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           9443,
				WebhookPath:             &path,
				AdmissionReviewVersions: []string{"v1"},
				TargetPort:              &intstr.IntOrString{Type: intstr.Int, IntVal: 9443},
			},
		}
	})

	data, err := generateWebhookServices(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "kind: Service")
	assert.Contains(t, s, "port: 9443")
}

func TestGenerateCertProvider(t *testing.T) {
	path := "/validate"
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
			{
				Type:                    v1alpha1.ValidatingAdmissionWebhook,
				GenerateName:            "validate-test",
				DeploymentName:          "controller-manager",
				ContainerPort:           443,
				WebhookPath:             &path,
				AdmissionReviewVersions: []string{"v1"},
			},
		}
	})

	data, err := generateCertProvider(b)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "kind: Issuer")
	assert.Contains(t, s, "kind: Certificate")
	assert.Contains(t, s, "cert-manager")
}

func TestGenerateCertProvider_NoWebhooks(t *testing.T) {
	b := makeMinimalBundle()
	data, err := generateCertProvider(b)
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestGenerateAdditional(t *testing.T) {
	t.Run("NamespacedResource", func(t *testing.T) {
		b := makeMinimalBundle(func(b *bundle.RegistryV1) {
			b.Others = []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "test-config",
							"namespace": "original-ns",
						},
						"data": map[string]interface{}{
							"key": "value",
						},
					},
				},
			}
		})

		data, err := generateAdditional(b)
		require.NoError(t, err)
		s := string(data)
		assert.Contains(t, s, "{{ .Release.Namespace }}")
	})

	t.Run("UnsupportedKind", func(t *testing.T) {
		b := makeMinimalBundle(func(b *bundle.RegistryV1) {
			b.Others = []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]interface{}{
							"name": "test-pod",
						},
					},
				},
			}
		})

		_, err := generateAdditional(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported resource")
	})
}

func TestGenerateAdditional_Empty(t *testing.T) {
	b := makeMinimalBundle()
	data, err := generateAdditional(b)
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestGenerateSchema_AllNamespacesOnly(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeAllNamespaces)
	data, err := generateSchema(b, modes, false)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	wnSchema := props["watchNamespace"].(map[string]interface{})
	assert.Equal(t, "", wnSchema["const"])
}

func TestGenerateSchema_NonAllNamespacesOnly(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeSingleNamespace)
	data, err := generateSchema(b, modes, false)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	wnSchema := props["watchNamespace"].(map[string]interface{})
	assert.Equal(t, float64(1), wnSchema["minLength"])

	// watchNamespace should be required
	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "watchNamespace")
}

func TestGenerateSchema_WithWebhooks(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeAllNamespaces)
	data, err := generateSchema(b, modes, true)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	certSchema := props["certProvider"].(map[string]interface{})
	assert.Equal(t, "string", certSchema["type"])

	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "certProvider")
}

func TestGenerateHelpers(t *testing.T) {
	data := generateHelpers()
	require.NotEmpty(t, data)
	s := string(data)
	assert.Contains(t, s, "olmTargetNamespaces")
	assert.Contains(t, s, "mergeEnv")
}

// ---- Tests for utility functions ----

func TestEscapeHelm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"NoEscape", "hello world", "hello world"},
		{"EscapeBraces", "value: {{something}}", "value: {{ `{{` }}something}}"},
		{"MultipleBraces", "a: {{b}} c: {{d}}", "a: {{ `{{` }}b}} c: {{ `{{` }}d}}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, escapeHelm(tc.input))
		})
	}
}

func TestWriteYAMLField(t *testing.T) {
	t.Run("Scalar", func(t *testing.T) {
		var sb strings.Builder
		writeYAMLField(&sb, "enabled", 0, true)
		assert.Equal(t, "enabled:\n  true\n", sb.String())
	})

	t.Run("ScalarIndented", func(t *testing.T) {
		var sb strings.Builder
		writeYAMLField(&sb, "count", 4, 42)
		assert.Equal(t, "    count:\n      42\n", sb.String())
	})

	t.Run("SingleKeyMap", func(t *testing.T) {
		var sb strings.Builder
		writeYAMLField(&sb, "myField", 0, map[string]string{"key": "value"})
		assert.Equal(t, "myField:\n  key: value\n", sb.String())
	})

	t.Run("MultilineMap", func(t *testing.T) {
		var sb strings.Builder
		obj := map[string]interface{}{
			"a": "1",
			"b": "2",
		}
		writeYAMLField(&sb, "data", 2, obj)
		result := sb.String()
		assert.True(t, strings.HasPrefix(result, "  data:\n"))
		for _, line := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
			if line != "" && !strings.HasSuffix(line, ":") {
				assert.True(t, strings.HasPrefix(line, "    "), "line %q should be indented by 4", line)
			}
		}
	})
}

func TestEscapeYAMLString(t *testing.T) {
	t.Run("PlainString", func(t *testing.T) {
		assert.Equal(t, "hello", escapeYAMLString("hello"))
	})

	t.Run("SpecialCharsQuoted", func(t *testing.T) {
		result := escapeYAMLString("value: with colon")
		assert.True(t, strings.HasPrefix(result, `"`), "should be quoted: %s", result)
	})

	t.Run("BooleanStringsQuoted", func(t *testing.T) {
		for _, s := range []string{"true", "false", "True", "False", "TRUE", "FALSE", "yes", "no", "Yes", "No", "on", "off", "On", "Off"} {
			result := escapeYAMLString(s)
			assert.Equal(t, fmt.Sprintf("%q", s), result, "boolean-like %q should be quoted", s)
		}
	})

	t.Run("NullStringsQuoted", func(t *testing.T) {
		for _, s := range []string{"null", "Null", "NULL", "~"} {
			result := escapeYAMLString(s)
			assert.Equal(t, fmt.Sprintf("%q", s), result, "null-like %q should be quoted", s)
		}
	})

	t.Run("NumericStringsQuoted", func(t *testing.T) {
		for _, s := range []string{"0", "42", "3.14", "-1", "+1", ".5"} {
			result := escapeYAMLString(s)
			assert.Equal(t, fmt.Sprintf("%q", s), result, "numeric-like %q should be quoted", s)
		}
	})
}

func TestCertNameForDeployment(t *testing.T) {
	t.Run("BasicName", func(t *testing.T) {
		name := certNameForDeployment("controller-manager")
		assert.Contains(t, name, "controller-manager")
		assert.Contains(t, name, "cert")
	})

	t.Run("DotsReplacedWithHyphens", func(t *testing.T) {
		name := certNameForDeployment("my.webhook.deployment")
		assert.NotContains(t, name, ".")
	})
}

func TestServiceNameForDeployment(t *testing.T) {
	name := serviceNameForDeployment("controller-manager")
	assert.Contains(t, name, "controller-manager")
	assert.Contains(t, name, "service")
}

func TestSaNameOrDefault(t *testing.T) {
	t.Run("NonEmpty", func(t *testing.T) {
		assert.Equal(t, "my-sa", saNameOrDefault("my-sa"))
	})
	t.Run("Empty", func(t *testing.T) {
		assert.Equal(t, "default", saNameOrDefault(""))
	})
}

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

func TestGetWebhookServicePort(t *testing.T) {
	t.Run("DefaultPort", func(t *testing.T) {
		wh := v1alpha1.WebhookDescription{}
		port := getWebhookServicePort(wh)
		assert.Equal(t, int32(443), port.Port)
	})

	t.Run("CustomPort", func(t *testing.T) {
		wh := v1alpha1.WebhookDescription{ContainerPort: 9443}
		port := getWebhookServicePort(wh)
		assert.Equal(t, int32(9443), port.Port)
	})

	t.Run("CustomTargetPort", func(t *testing.T) {
		tp := intstr.FromInt32(8443)
		wh := v1alpha1.WebhookDescription{
			ContainerPort: 9443,
			TargetPort:    &tp,
		}
		port := getWebhookServicePort(wh)
		assert.Equal(t, int32(9443), port.Port)
		assert.Equal(t, int32(8443), port.TargetPort.IntVal)
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
