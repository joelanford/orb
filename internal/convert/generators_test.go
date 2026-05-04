package convert

import (
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/orb/internal/bundle"
)

func defaultTestOpts() Options {
	return Options{
		InstallNamespace:    "operator-ns",
		TargetNamespaces:    []string{"target-ns"},
		UniqueNameGenerator: DefaultUniqueNameGenerator,
	}
}

func TestBundleCSVDeploymentGenerator(t *testing.T) {
	tests := []struct {
		name      string
		rv1       *bundle.RegistryV1
		opts      Options
		wantErr   bool
		wantCount int
		verify    func(t *testing.T, objs []interface{ GetName() string })
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "single deployment",
			rv1: &bundle.RegistryV1{
				PackageName: "test-pkg",
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test-csv",
						Annotations: map[string]string{"csv-annotation": "value"},
					},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
									{
										Name: "my-deployment",
										Spec: appsv1.DeploymentSpec{
											Template: corev1.PodTemplateSpec{
												ObjectMeta: metav1.ObjectMeta{
													Annotations: map[string]string{"pod-annotation": "pod-value"},
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
			opts:      defaultTestOpts(),
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleCSVDeploymentGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantCount)

			if tt.name == "single deployment" {
				dep, ok := objs[0].(*appsv1.Deployment)
				require.True(t, ok)
				assert.Equal(t, "my-deployment", dep.Name)
				assert.Equal(t, "operator-ns", dep.Namespace)
				assert.Equal(t, "value", dep.Spec.Template.Annotations["csv-annotation"])
				assert.Equal(t, "pod-value", dep.Spec.Template.Annotations["pod-annotation"])
				assert.Equal(t, "target-ns", dep.Spec.Template.Annotations["olm.targetNamespaces"])
				require.NotNil(t, dep.Spec.RevisionHistoryLimit)
				assert.Equal(t, int32(1), *dep.Spec.RevisionHistoryLimit)
			}
		})
	}
}

func TestBundleCSVPermissionsGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantNil bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "single permission single namespace",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "test-csv"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								Permissions: []v1alpha1.StrategyDeploymentPermissions{
									{
										ServiceAccountName: "my-sa",
										Rules: []rbacv1.PolicyRule{
											{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"pods"}},
										},
									},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 2, // Role + RoleBinding
		},
		{
			name: "AllNamespaces returns nil",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "test-csv"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								Permissions: []v1alpha1.StrategyDeploymentPermissions{
									{
										ServiceAccountName: "my-sa",
										Rules: []rbacv1.PolicyRule{
											{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"pods"}},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: Options{
				InstallNamespace:    "operator-ns",
				TargetNamespaces:    []string{""},
				UniqueNameGenerator: DefaultUniqueNameGenerator,
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleCSVPermissionsGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, objs)
				return
			}
			require.Len(t, objs, tt.wantLen)

			// Verify Role
			role, ok := objs[0].(*rbacv1.Role)
			require.True(t, ok)
			assert.Equal(t, "target-ns", role.Namespace)
			assert.Len(t, role.Rules, 1)

			// Verify RoleBinding
			rb, ok := objs[1].(*rbacv1.RoleBinding)
			require.True(t, ok)
			assert.Equal(t, "target-ns", rb.Namespace)
			require.Len(t, rb.Subjects, 1)
			assert.Equal(t, "my-sa", rb.Subjects[0].Name)
			assert.Equal(t, "operator-ns", rb.Subjects[0].Namespace)
		})
	}
}

func TestBundleCSVClusterPermissionsGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "single cluster permission",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "test-csv"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								ClusterPermissions: []v1alpha1.StrategyDeploymentPermissions{
									{
										ServiceAccountName: "cluster-sa",
										Rules: []rbacv1.PolicyRule{
											{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"nodes"}},
										},
									},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 2, // ClusterRole + ClusterRoleBinding
		},
		{
			name: "AllNamespaces promotes permissions to cluster permissions",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "test-csv"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								Permissions: []v1alpha1.StrategyDeploymentPermissions{
									{
										ServiceAccountName: "ns-sa",
										Rules: []rbacv1.PolicyRule{
											{Verbs: []string{"list"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
										},
									},
								},
								ClusterPermissions: []v1alpha1.StrategyDeploymentPermissions{
									{
										ServiceAccountName: "cluster-sa",
										Rules: []rbacv1.PolicyRule{
											{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"nodes"}},
										},
									},
								},
							},
						},
					},
				},
			},
			opts: Options{
				InstallNamespace:    "operator-ns",
				TargetNamespaces:    []string{""},
				UniqueNameGenerator: DefaultUniqueNameGenerator,
			},
			wantLen: 4, // 2 ClusterRole + 2 ClusterRoleBinding (original cluster perm + promoted perm)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleCSVClusterPermissionsGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "single cluster permission" {
				cr, ok := objs[0].(*rbacv1.ClusterRole)
				require.True(t, ok)
				assert.Len(t, cr.Rules, 1)

				crb, ok := objs[1].(*rbacv1.ClusterRoleBinding)
				require.True(t, ok)
				require.Len(t, crb.Subjects, 1)
				assert.Equal(t, "cluster-sa", crb.Subjects[0].Name)
				assert.Equal(t, "operator-ns", crb.Subjects[0].Namespace)
			}

			if tt.name == "AllNamespaces promotes permissions to cluster permissions" {
				// The original cluster permission comes first, then the promoted permission
				cr1, ok := objs[0].(*rbacv1.ClusterRole)
				require.True(t, ok)
				assert.Len(t, cr1.Rules, 1)

				// The promoted permission should have the original rule + namespace get/list/watch
				cr2, ok := objs[2].(*rbacv1.ClusterRole)
				require.True(t, ok)
				assert.Len(t, cr2.Rules, 2) // original rule + namespace rule
			}
		})
	}
}

func TestBundleCSVServiceAccountGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "named SA created",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								Permissions: []v1alpha1.StrategyDeploymentPermissions{
									{ServiceAccountName: "my-sa"},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
		{
			name: "default SA skipped",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								Permissions: []v1alpha1.StrategyDeploymentPermissions{
									{ServiceAccountName: "default"},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleCSVServiceAccountGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "named SA created" {
				sa, ok := objs[0].(*corev1.ServiceAccount)
				require.True(t, ok)
				assert.Equal(t, "my-sa", sa.Name)
				assert.Equal(t, "operator-ns", sa.Namespace)
			}
		})
	}
}

func TestBundleCRDGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "simple CRD passthrough",
			rv1: &bundle.RegistryV1{
				CRDs: []apiextensionsv1.CustomResourceDefinition{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
						Spec: apiextensionsv1.CustomResourceDefinitionSpec{
							Group: "example.com",
							Names: apiextensionsv1.CustomResourceDefinitionNames{
								Kind:   "Foo",
								Plural: "foos",
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleCRDGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "simple CRD passthrough" {
				crd, ok := objs[0].(*apiextensionsv1.CustomResourceDefinition)
				require.True(t, ok)
				assert.Equal(t, "foos.example.com", crd.Name)
			}
		})
	}
}

func TestBundleAdditionalResourcesGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "supported namespaced resource gets namespace",
			rv1: &bundle.RegistryV1{
				Others: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name": "my-config",
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleAdditionalResourcesGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "supported namespaced resource gets namespace" {
				assert.Equal(t, "operator-ns", objs[0].GetNamespace())
				assert.Equal(t, "my-config", objs[0].GetName())
			}
		})
	}
}

func TestBundleValidatingWebhookResourceGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "validating webhook creates config",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						WebhookDefinitions: []v1alpha1.WebhookDescription{
							{
								Type:                    v1alpha1.ValidatingAdmissionWebhook,
								GenerateName:            "validate.example.com-",
								DeploymentName:          "webhook-deploy",
								ContainerPort:           443,
								AdmissionReviewVersions: []string{"v1"},
							},
						},
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
									{Name: "webhook-deploy"},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleValidatingWebhookResourceGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "validating webhook creates config" {
				assert.Equal(t, "validate.example.com", objs[0].GetName())
			}
		})
	}
}

func TestBundleMutatingWebhookResourceGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "mutating webhook creates config",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						WebhookDefinitions: []v1alpha1.WebhookDescription{
							{
								Type:                    v1alpha1.MutatingAdmissionWebhook,
								GenerateName:            "mutate.example.com-",
								DeploymentName:          "webhook-deploy",
								ContainerPort:           443,
								AdmissionReviewVersions: []string{"v1"},
							},
						},
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
									{Name: "webhook-deploy"},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleMutatingWebhookResourceGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "mutating webhook creates config" {
				assert.Equal(t, "mutate.example.com", objs[0].GetName())
			}
		})
	}
}

func TestBundleDeploymentServiceResourceGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantErr bool
		wantLen int
	}{
		{
			name:    "nil bundle returns error",
			rv1:     nil,
			opts:    defaultTestOpts(),
			wantErr: true,
		},
		{
			name: "webhook deployment creates service",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						WebhookDefinitions: []v1alpha1.WebhookDescription{
							{
								Type:           v1alpha1.ValidatingAdmissionWebhook,
								GenerateName:   "validate.example.com-",
								DeploymentName: "webhook-deploy",
								ContainerPort:  443,
							},
						},
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
									{
										Name: "webhook-deploy",
										Spec: appsv1.DeploymentSpec{
											Selector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"app": "webhook"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := BundleDeploymentServiceResourceGenerator(tt.rv1, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, objs, tt.wantLen)

			if tt.name == "webhook deployment creates service" {
				svc, ok := objs[0].(*corev1.Service)
				require.True(t, ok)
				assert.Equal(t, "operator-ns", svc.Namespace)
				assert.NotEmpty(t, svc.Spec.Ports)
				assert.Equal(t, map[string]string{"app": "webhook"}, svc.Spec.Selector)
			}
		})
	}
}

func TestCertProviderResourceGenerator(t *testing.T) {
	tests := []struct {
		name    string
		rv1     *bundle.RegistryV1
		opts    Options
		wantLen int
	}{
		{
			name: "no webhooks returns empty",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 0,
		},
		{
			name: "with webhooks and no provider returns empty",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						WebhookDefinitions: []v1alpha1.WebhookDescription{
							{
								Type:           v1alpha1.ValidatingAdmissionWebhook,
								GenerateName:   "validate.example.com-",
								DeploymentName: "webhook-deploy",
								ContainerPort:  443,
							},
						},
					},
				},
			},
			opts:    defaultTestOpts(),
			wantLen: 0,
		},
		{
			name: "with webhooks and provider generates cert objects",
			rv1: &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						WebhookDefinitions: []v1alpha1.WebhookDescription{
							{
								Type:           v1alpha1.ValidatingAdmissionWebhook,
								GenerateName:   "validate.example.com-",
								DeploymentName: "webhook-deploy",
								ContainerPort:  443,
							},
						},
					},
				},
			},
			opts: Options{
				InstallNamespace:    "operator-ns",
				TargetNamespaces:    []string{"target-ns"},
				UniqueNameGenerator: DefaultUniqueNameGenerator,
				CertificateProvider: &fakeCertProvider{
					additionalObjects: []unstructured.Unstructured{
						{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{"name": "cert-secret"}}},
					},
				},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := CertProviderResourceGenerator(tt.rv1, tt.opts)
			require.NoError(t, err)
			assert.Len(t, objs, tt.wantLen)
		})
	}
}

// fakeCertProvider implements CertificateProvider for testing
type fakeCertProvider struct {
	additionalObjects []unstructured.Unstructured
	certSecretInfo    CertSecretInfo
}

func (f *fakeCertProvider) InjectCABundle(_ client.Object, _ CertificateProvisionerConfig) error {
	return nil
}

func (f *fakeCertProvider) AdditionalObjects(_ CertificateProvisionerConfig) ([]unstructured.Unstructured, error) {
	return f.additionalObjects, nil
}

func (f *fakeCertProvider) GetCertSecretInfo(_ CertificateProvisionerConfig) CertSecretInfo {
	return f.certSecretInfo
}

func TestSaNameOrDefault(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "non-empty returns input",
			input: "my-sa",
			want:  "my-sa",
		},
		{
			name:  "empty returns default",
			input: "",
			want:  "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, saNameOrDefault(tt.input))
		})
	}
}

func TestGetWebhookServicePort(t *testing.T) {
	tests := []struct {
		name string
		wh   v1alpha1.WebhookDescription
		want corev1.ServicePort
	}{
		{
			name: "default port 443",
			wh:   v1alpha1.WebhookDescription{},
			want: corev1.ServicePort{
				Name:       "443",
				Port:       443,
				TargetPort: intstr.FromInt32(443),
			},
		},
		{
			name: "custom port",
			wh:   v1alpha1.WebhookDescription{ContainerPort: 9443},
			want: corev1.ServicePort{
				Name:       "9443",
				Port:       9443,
				TargetPort: intstr.FromInt32(9443),
			},
		},
		{
			name: "custom target port",
			wh: v1alpha1.WebhookDescription{
				ContainerPort: 8443,
				TargetPort:    ptrIntStr(intstr.FromInt32(9999)),
			},
			want: corev1.ServicePort{
				Name:       "8443",
				Port:       8443,
				TargetPort: intstr.FromInt32(9999),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getWebhookServicePort(tt.wh)
			assert.Equal(t, tt.want, got)
		})
	}
}

func ptrIntStr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}

func TestGetWebhookNamespaceSelector(t *testing.T) {
	tests := []struct {
		name             string
		targetNamespaces []string
		wantNil          bool
	}{
		{
			name:             "specific namespaces returns selector",
			targetNamespaces: []string{"ns1", "ns2"},
			wantNil:          false,
		},
		{
			name:             "AllNamespaces returns nil",
			targetNamespaces: []string{""},
			wantNil:          true,
		},
		{
			name:             "empty slice returns nil",
			targetNamespaces: []string{},
			wantNil:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getWebhookNamespaceSelector(tt.targetNamespaces)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				require.Len(t, got.MatchExpressions, 1)
				assert.Equal(t, "kubernetes.io/metadata.name", got.MatchExpressions[0].Key)
				assert.Equal(t, metav1.LabelSelectorOpIn, got.MatchExpressions[0].Operator)
				assert.Equal(t, tt.targetNamespaces, got.MatchExpressions[0].Values)
			}
		})
	}
}
