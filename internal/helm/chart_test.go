package helm

import (
	"fmt"
	"strings"
	"testing"

	bsemver "github.com/blang/semver/v4"
	opversion "github.com/operator-framework/api/pkg/lib/version"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

// renderChart generates a chart from the bundle and renders all templates.
// Returns an error if rendering fails (e.g., due to {{ fail }}).
func renderChart(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) (map[string]string, error) {
	t.Helper()

	c, err := Generate(b)
	require.NoError(t, err)

	if _, ok := valOverrides["deploymentConfig"]; !ok {
		valOverrides["deploymentConfig"] = map[string]any{}
	}

	vals := map[string]any{
		"Release": map[string]any{
			"Name":      "test-release",
			"Namespace": "test-ns",
			"IsInstall": true,
		},
		"Values": valOverrides,
	}
	coalesced, err := util.CoalesceValues(c, vals)
	require.NoError(t, err)

	return engine.Render(c, coalesced)
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

func TestWriteYAMLFieldRaw_PanicsOnMarshalError(t *testing.T) {
	assert.Panics(t, func() {
		var sb strings.Builder
		// Channels cannot be marshaled to YAML.
		writeYAMLFieldRaw(&sb, 0, make(chan int))
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
