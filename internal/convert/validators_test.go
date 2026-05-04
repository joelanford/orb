package convert

import (
	"strings"
	"testing"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type bundleOption func(*registryv1.Bundle)

func makeBundle(opts ...bundleOption) *registryv1.Bundle {
	b := &registryv1.Bundle{}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func withPackageName(name string) bundleOption {
	return func(b *registryv1.Bundle) {
		b.PackageName = name
	}
}

func withDeploymentSpecs(names ...string) bundleOption {
	return func(b *registryv1.Bundle) {
		for _, name := range names {
			b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = append(
				b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs,
				v1alpha1.StrategyDeploymentSpec{Name: name},
			)
		}
	}
}

func withCRDs(names ...string) bundleOption {
	return func(b *registryv1.Bundle) {
		for _, name := range names {
			b.CRDs = append(b.CRDs, apiextensionsv1.CustomResourceDefinition{
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Singular: name,
					},
				},
			})
			b.CRDs[len(b.CRDs)-1].Name = name
		}
	}
}

func withOwnedCRDs(names ...string) bundleOption {
	return func(b *registryv1.Bundle) {
		for _, name := range names {
			b.CSV.Spec.CustomResourceDefinitions.Owned = append(
				b.CSV.Spec.CustomResourceDefinitions.Owned,
				v1alpha1.CRDDescription{Name: name},
			)
		}
	}
}

func withInstallModes(modes ...v1alpha1.InstallModeType) bundleOption {
	return func(b *registryv1.Bundle) {
		for _, mode := range modes {
			b.CSV.Spec.InstallModes = append(b.CSV.Spec.InstallModes, v1alpha1.InstallMode{
				Type:      mode,
				Supported: true,
			})
		}
	}
}

func withWebhookDefinitions(whs ...v1alpha1.WebhookDescription) bundleOption {
	return func(b *registryv1.Bundle) {
		b.CSV.Spec.WebhookDefinitions = append(b.CSV.Spec.WebhookDefinitions, whs...)
	}
}

func TestCheckDeploymentSpecUniqueness(t *testing.T) {
	tests := []struct {
		name      string
		bundle    *registryv1.Bundle
		wantErrs  int
		wantSubst []string
	}{
		{
			name:     "no deployments",
			bundle:   makeBundle(),
			wantErrs: 0,
		},
		{
			name:     "unique names",
			bundle:   makeBundle(withDeploymentSpecs("dep-a", "dep-b", "dep-c")),
			wantErrs: 0,
		},
		{
			name:      "duplicate name",
			bundle:    makeBundle(withDeploymentSpecs("dep-a", "dep-a")),
			wantErrs:  1,
			wantSubst: []string{"dep-a"},
		},
		{
			name:      "multiple duplicates",
			bundle:    makeBundle(withDeploymentSpecs("dep-a", "dep-b", "dep-a", "dep-b")),
			wantErrs:  2,
			wantSubst: []string{"dep-a", "dep-b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckDeploymentSpecUniqueness(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
			for i, substr := range tc.wantSubst {
				assert.ErrorContains(t, errs[i], substr)
			}
		})
	}
}

func TestCheckDeploymentNameIsDNS1123SubDomain(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name:     "valid name",
			bundle:   makeBundle(withDeploymentSpecs("valid-name")),
			wantErrs: 0,
		},
		{
			name:     "uppercase invalid",
			bundle:   makeBundle(withDeploymentSpecs("Invalid-Name")),
			wantErrs: 1,
		},
		{
			name:     "too long",
			bundle:   makeBundle(withDeploymentSpecs(strings.Repeat("a", 254))),
			wantErrs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckDeploymentNameIsDNS1123SubDomain(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckOwnedCRDExistence(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name:     "owned CRD exists in bundle",
			bundle:   makeBundle(withCRDs("foos.example.com"), withOwnedCRDs("foos.example.com")),
			wantErrs: 0,
		},
		{
			name:     "owned CRD missing from bundle",
			bundle:   makeBundle(withOwnedCRDs("missing.example.com")),
			wantErrs: 1,
		},
		{
			name:     "no owned CRDs",
			bundle:   makeBundle(withCRDs("foos.example.com")),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckOwnedCRDExistence(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckCRDResourceUniqueness(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name:     "unique CRDs",
			bundle:   makeBundle(withCRDs("foos.example.com", "bars.example.com")),
			wantErrs: 0,
		},
		{
			name:     "duplicate CRD",
			bundle:   makeBundle(withCRDs("foos.example.com", "foos.example.com")),
			wantErrs: 1,
		},
		{
			name:     "empty",
			bundle:   makeBundle(),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckCRDResourceUniqueness(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckPackageNameNotEmpty(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name:     "non-empty",
			bundle:   makeBundle(withPackageName("my-operator")),
			wantErrs: 0,
		},
		{
			name:     "empty",
			bundle:   makeBundle(),
			wantErrs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckPackageNameNotEmpty(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckConversionWebhookSupport(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "no conversion webhooks",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
				}),
				withInstallModes(v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeAllNamespaces),
			),
			wantErrs: 0,
		},
		{
			name: "AllNamespaces only with conversion webhook",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "convert.example.com",
					Type:         v1alpha1.ConversionWebhook,
				}),
				withInstallModes(v1alpha1.InstallModeTypeAllNamespaces),
			),
			wantErrs: 0,
		},
		{
			name: "mixed install modes with conversion webhook",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "convert.example.com",
					Type:         v1alpha1.ConversionWebhook,
				}),
				withInstallModes(v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeAllNamespaces),
			),
			wantErrs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckConversionWebhookSupport(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckWebhookDeploymentReferentialIntegrity(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "valid reference",
			bundle: makeBundle(
				withDeploymentSpecs("my-deploy"),
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName:   "validate.example.com",
					Type:           v1alpha1.ValidatingAdmissionWebhook,
					DeploymentName: "my-deploy",
				}),
			),
			wantErrs: 0,
		},
		{
			name: "non-existent deployment",
			bundle: makeBundle(
				withDeploymentSpecs("existing-deploy"),
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName:   "validate.example.com",
					Type:           v1alpha1.ValidatingAdmissionWebhook,
					DeploymentName: "missing-deploy",
				}),
			),
			wantErrs: 1,
		},
		{
			name:     "no webhooks",
			bundle:   makeBundle(withDeploymentSpecs("my-deploy")),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckWebhookDeploymentReferentialIntegrity(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckWebhookNameUniqueness(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "unique per type",
			bundle: makeBundle(
				withWebhookDefinitions(
					v1alpha1.WebhookDescription{GenerateName: "wh-a", Type: v1alpha1.ValidatingAdmissionWebhook},
					v1alpha1.WebhookDescription{GenerateName: "wh-b", Type: v1alpha1.ValidatingAdmissionWebhook},
				),
			),
			wantErrs: 0,
		},
		{
			name: "duplicate same type",
			bundle: makeBundle(
				withWebhookDefinitions(
					v1alpha1.WebhookDescription{GenerateName: "wh-a", Type: v1alpha1.ValidatingAdmissionWebhook},
					v1alpha1.WebhookDescription{GenerateName: "wh-a", Type: v1alpha1.ValidatingAdmissionWebhook},
				),
			),
			wantErrs: 1,
		},
		{
			name: "same name different types is ok",
			bundle: makeBundle(
				withWebhookDefinitions(
					v1alpha1.WebhookDescription{GenerateName: "wh-a", Type: v1alpha1.ValidatingAdmissionWebhook},
					v1alpha1.WebhookDescription{GenerateName: "wh-a", Type: v1alpha1.MutatingAdmissionWebhook},
				),
			),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckWebhookNameUniqueness(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckConversionWebhooksReferenceOwnedCRDs(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "references owned CRD",
			bundle: makeBundle(
				withOwnedCRDs("foos.example.com"),
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName:   "convert.example.com",
					Type:           v1alpha1.ConversionWebhook,
					ConversionCRDs: []string{"foos.example.com"},
				}),
			),
			wantErrs: 0,
		},
		{
			name: "references non-owned CRD",
			bundle: makeBundle(
				withOwnedCRDs("foos.example.com"),
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName:   "convert.example.com",
					Type:           v1alpha1.ConversionWebhook,
					ConversionCRDs: []string{"bars.example.com"},
				}),
			),
			wantErrs: 1,
		},
		{
			name: "no conversion webhooks",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
				}),
			),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckConversionWebhooksReferenceOwnedCRDs(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckConversionWebhookCRDReferenceUniqueness(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "each CRD referenced by one webhook",
			bundle: makeBundle(
				withWebhookDefinitions(
					v1alpha1.WebhookDescription{
						GenerateName:   "convert-a",
						Type:           v1alpha1.ConversionWebhook,
						ConversionCRDs: []string{"foos.example.com"},
					},
					v1alpha1.WebhookDescription{
						GenerateName:   "convert-b",
						Type:           v1alpha1.ConversionWebhook,
						ConversionCRDs: []string{"bars.example.com"},
					},
				),
			),
			wantErrs: 0,
		},
		{
			name: "CRD referenced by two webhooks",
			bundle: makeBundle(
				withWebhookDefinitions(
					v1alpha1.WebhookDescription{
						GenerateName:   "convert-a",
						Type:           v1alpha1.ConversionWebhook,
						ConversionCRDs: []string{"foos.example.com"},
					},
					v1alpha1.WebhookDescription{
						GenerateName:   "convert-b",
						Type:           v1alpha1.ConversionWebhook,
						ConversionCRDs: []string{"foos.example.com"},
					},
				),
			),
			wantErrs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckConversionWebhookCRDReferenceUniqueness(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckWebhookNameIsDNS1123SubDomain(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "valid name",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "valid-webhook.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
				}),
			),
			wantErrs: 0,
		},
		{
			name: "invalid name",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "INVALID_NAME",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
				}),
			),
			wantErrs: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckWebhookNameIsDNS1123SubDomain(tc.bundle)
			assert.Len(t, errs, tc.wantErrs)
		})
	}
}

func TestCheckWebhookRules(t *testing.T) {
	tests := []struct {
		name     string
		bundle   *registryv1.Bundle
		wantErrs int
	}{
		{
			name: "valid rules",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Rule: admissionregistrationv1.Rule{
								APIGroups: []string{"apps"},
								Resources: []string{"deployments"},
							},
						},
					},
				}),
			),
			wantErrs: 0,
		},
		{
			name: "forbidden api group wildcard",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Rule: admissionregistrationv1.Rule{
								APIGroups: []string{"*"},
								Resources: []string{"pods"},
							},
						},
					},
				}),
			),
			wantErrs: 1,
		},
		{
			name: "forbidden olm api group",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Rule: admissionregistrationv1.Rule{
								APIGroups: []string{"olm.operatorframework.io"},
								Resources: []string{"operators"},
							},
						},
					},
				}),
			),
			wantErrs: 1,
		},
		{
			name: "forbidden admissionregistration resource",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "validate.example.com",
					Type:         v1alpha1.ValidatingAdmissionWebhook,
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Rule: admissionregistrationv1.Rule{
								APIGroups: []string{"admissionregistration.k8s.io"},
								Resources: []string{"mutatingwebhookconfigurations"},
							},
						},
					},
				}),
			),
			wantErrs: 1,
		},
		{
			name: "conversion webhook skipped",
			bundle: makeBundle(
				withWebhookDefinitions(v1alpha1.WebhookDescription{
					GenerateName: "convert.example.com",
					Type:         v1alpha1.ConversionWebhook,
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Rule: admissionregistrationv1.Rule{
								APIGroups: []string{"*"},
								Resources: []string{"pods"},
							},
						},
					},
				}),
			),
			wantErrs: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := CheckWebhookRules(tc.bundle)
			require.Len(t, errs, tc.wantErrs)
		})
	}
}
