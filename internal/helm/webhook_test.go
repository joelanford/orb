package helm

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

// makeWebhookBundle returns a bundle with the given webhook definitions.
func makeWebhookBundle(webhooks []v1alpha1.WebhookDescription) *bundle.RegistryV1 {
	return makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = []v1alpha1.StrategyDeploymentSpec{
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
								{Name: "manager", Image: "registry.io/test-operator:v1.0.0"},
							},
						},
					},
				},
			},
		}
		b.CSV.Spec.WebhookDefinitions = webhooks
	})
}

// renderWebhookConfigs generates a helm chart from the bundle, renders the
// webhook.yaml template with the given values, and parses the result into
// ValidatingWebhookConfiguration and MutatingWebhookConfiguration slices.
func renderWebhookConfigs(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) (
	[]admissionregistrationv1.ValidatingWebhookConfiguration,
	[]admissionregistrationv1.MutatingWebhookConfiguration,
) {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var webhookYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "webhook.yaml") {
			webhookYAML = data
			break
		}
	}

	var vwcs []admissionregistrationv1.ValidatingWebhookConfiguration
	var mwcs []admissionregistrationv1.MutatingWebhookConfiguration

	if webhookYAML == "" {
		return vwcs, mwcs
	}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(webhookYAML), 4096)
	for {
		var vwc admissionregistrationv1.ValidatingWebhookConfiguration
		if err := decoder.Decode(&vwc); err != nil {
			break
		}
		if vwc.Kind == "ValidatingWebhookConfiguration" {
			vwcs = append(vwcs, vwc)
			continue
		}
		// Re-decode as mutating
		decoder2 := yaml.NewYAMLOrJSONDecoder(strings.NewReader(webhookYAML), 4096)
		for {
			var mwc admissionregistrationv1.MutatingWebhookConfiguration
			if err := decoder2.Decode(&mwc); err != nil {
				break
			}
			if mwc.Kind == "MutatingWebhookConfiguration" {
				mwcs = append(mwcs, mwc)
			}
		}
		break
	}

	// If the first pass didn't find validating, try again properly parsing each doc
	if len(vwcs) == 0 && len(mwcs) == 0 {
		// Parse each document individually
		docs := strings.Split(webhookYAML, "---")
		for _, doc := range docs {
			doc = strings.TrimSpace(doc)
			if doc == "" {
				continue
			}
			if strings.Contains(doc, "kind: ValidatingWebhookConfiguration") {
				var vwc admissionregistrationv1.ValidatingWebhookConfiguration
				d := yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096)
				if err := d.Decode(&vwc); err == nil && vwc.Kind == "ValidatingWebhookConfiguration" {
					vwcs = append(vwcs, vwc)
				}
			} else if strings.Contains(doc, "kind: MutatingWebhookConfiguration") {
				var mwc admissionregistrationv1.MutatingWebhookConfiguration
				d := yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096)
				if err := d.Decode(&mwc); err == nil && mwc.Kind == "MutatingWebhookConfiguration" {
					mwcs = append(mwcs, mwc)
				}
			}
		}
	}

	return vwcs, mwcs
}

// renderWebhookServices generates a helm chart from the bundle, renders the
// service.yaml template with the given values, and parses the result into
// Service slices.
func renderWebhookServices(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) []corev1.Service {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var svcYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "service.yaml") {
			svcYAML = data
			break
		}
	}
	require.NotEmpty(t, svcYAML, "service.yaml not found in rendered output")

	var svcs []corev1.Service
	docs := strings.Split(svcYAML, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var svc corev1.Service
		d := yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096)
		if err := d.Decode(&svc); err == nil && svc.Kind == "Service" {
			svcs = append(svcs, svc)
		}
	}
	return svcs
}

// --- Validating Webhook ---

func TestWebhook_Validating(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
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
	})

	vwcs, mwcs := renderWebhookConfigs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	require.Len(t, vwcs, 1)
	assert.Empty(t, mwcs)

	vwc := vwcs[0]
	assert.Equal(t, "ValidatingWebhookConfiguration", vwc.Kind)
	assert.Equal(t, "validate-test", vwc.Name)

	require.Len(t, vwc.Webhooks, 1)
	wh := vwc.Webhooks[0]
	assert.Equal(t, "validate-test", wh.Name)

	// clientConfig
	require.NotNil(t, wh.ClientConfig.Service)
	assert.Equal(t, serviceNameForDeployment("controller-manager"), wh.ClientConfig.Service.Name)
	assert.Equal(t, "test-ns", wh.ClientConfig.Service.Namespace)
	require.NotNil(t, wh.ClientConfig.Service.Path)
	assert.Equal(t, "/validate", *wh.ClientConfig.Service.Path)
	require.NotNil(t, wh.ClientConfig.Service.Port)
	assert.Equal(t, int32(443), *wh.ClientConfig.Service.Port)

	// rules
	require.Len(t, wh.Rules, 1)
	assert.Equal(t, []admissionregistrationv1.OperationType{admissionregistrationv1.Create}, wh.Rules[0].Operations)
	assert.Equal(t, []string{"example.com"}, wh.Rules[0].APIGroups)
	assert.Equal(t, []string{"v1"}, wh.Rules[0].APIVersions)
	assert.Equal(t, []string{"tests"}, wh.Rules[0].Resources)

	// failurePolicy, sideEffects, admissionReviewVersions
	require.NotNil(t, wh.FailurePolicy)
	assert.Equal(t, admissionregistrationv1.Fail, *wh.FailurePolicy)
	require.NotNil(t, wh.SideEffects)
	assert.Equal(t, admissionregistrationv1.SideEffectClassNone, *wh.SideEffects)
	assert.Equal(t, []string{"v1"}, wh.AdmissionReviewVersions)
}

// --- Mutating Webhook ---

func TestWebhook_Mutating(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/mutate"
	reinvocationPolicy := admissionregistrationv1.IfNeededReinvocationPolicy

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
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
	})

	vwcs, mwcs := renderWebhookConfigs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	assert.Empty(t, vwcs)
	require.Len(t, mwcs, 1)

	mwc := mwcs[0]
	assert.Equal(t, "MutatingWebhookConfiguration", mwc.Kind)
	assert.Equal(t, "mutate-test", mwc.Name)

	require.Len(t, mwc.Webhooks, 1)
	wh := mwc.Webhooks[0]
	assert.Equal(t, "mutate-test", wh.Name)

	// clientConfig
	require.NotNil(t, wh.ClientConfig.Service)
	assert.Equal(t, serviceNameForDeployment("controller-manager"), wh.ClientConfig.Service.Name)
	assert.Equal(t, "test-ns", wh.ClientConfig.Service.Namespace)
	require.NotNil(t, wh.ClientConfig.Service.Path)
	assert.Equal(t, "/mutate", *wh.ClientConfig.Service.Path)

	// reinvocationPolicy
	require.NotNil(t, wh.ReinvocationPolicy)
	assert.Equal(t, admissionregistrationv1.IfNeededReinvocationPolicy, *wh.ReinvocationPolicy)

	assert.Equal(t, []string{"v1"}, wh.AdmissionReviewVersions)
}

// --- Conversion Webhook (skipped) ---

func TestWebhook_SkipsConversion(t *testing.T) {
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

	vwcs, mwcs := renderWebhookConfigs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	assert.Empty(t, vwcs, "conversion webhooks should not emit a ValidatingWebhookConfiguration")
	assert.Empty(t, mwcs, "conversion webhooks should not emit a MutatingWebhookConfiguration")
}

// --- Cert Provider Variations ---

func TestWebhook_CertManagerAnnotation(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
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
	})

	vwcs, _ := renderWebhookConfigs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	require.Len(t, vwcs, 1)

	certName := certNameForDeployment("controller-manager")
	expectedAnnotation := "test-ns/" + certName
	assert.Equal(t, expectedAnnotation, vwcs[0].Annotations["cert-manager.io/inject-ca-from"])
}

func TestWebhook_NoCertProvider(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	path := "/validate"

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
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
	})

	vwcs, _ := renderWebhookConfigs(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "",
	})
	require.Len(t, vwcs, 1)

	// No cert-manager annotation when certProvider is empty
	assert.Empty(t, vwcs[0].Annotations)
}

// --- Webhook Services ---

func TestWebhookService_Rendered(t *testing.T) {
	path := "/validate"

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
		{
			Type:                    v1alpha1.ValidatingAdmissionWebhook,
			GenerateName:            "validate-test",
			DeploymentName:          "controller-manager",
			ContainerPort:           9443,
			WebhookPath:             &path,
			AdmissionReviewVersions: []string{"v1"},
			TargetPort:              &intstr.IntOrString{Type: intstr.Int, IntVal: 9443},
		},
	})

	svcs := renderWebhookServices(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	require.Len(t, svcs, 1)

	svc := svcs[0]
	assert.Equal(t, "Service", svc.Kind)
	assert.Equal(t, serviceNameForDeployment("controller-manager"), svc.Name)
	assert.Equal(t, "test-ns", svc.Namespace)

	// Selector
	assert.Equal(t, "test", svc.Spec.Selector["app"])

	// Ports
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(9443), svc.Spec.Ports[0].Port)
	assert.Equal(t, int32(9443), svc.Spec.Ports[0].TargetPort.IntVal)
}

func TestWebhookService_DefaultPort(t *testing.T) {
	path := "/validate"

	b := makeWebhookBundle([]v1alpha1.WebhookDescription{
		{
			Type:                    v1alpha1.ValidatingAdmissionWebhook,
			GenerateName:            "validate-test",
			DeploymentName:          "controller-manager",
			ContainerPort:           443,
			WebhookPath:             &path,
			AdmissionReviewVersions: []string{"v1"},
		},
	})

	svcs := renderWebhookServices(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	require.Len(t, svcs, 1)
	require.Len(t, svcs[0].Spec.Ports, 1)
	assert.Equal(t, int32(443), svcs[0].Spec.Ports[0].Port)
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
