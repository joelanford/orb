package convert

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/orb/internal/bundle"
)

func TestBundleValidator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       BundleValidator
		rv1     *bundle.RegistryV1
		wantErr bool
	}{
		{
			name: "no validators",
			v:    BundleValidator{},
			rv1:  &bundle.RegistryV1{},
		},
		{
			name: "passing validator",
			v: BundleValidator{
				func(_ *bundle.RegistryV1) []error { return nil },
			},
			rv1: &bundle.RegistryV1{},
		},
		{
			name: "failing validator",
			v: BundleValidator{
				func(_ *bundle.RegistryV1) []error { return []error{assert.AnError} },
			},
			rv1:     &bundle.RegistryV1{},
			wantErr: true,
		},
		{
			name: "mixed validators",
			v: BundleValidator{
				func(_ *bundle.RegistryV1) []error { return nil },
				func(_ *bundle.RegistryV1) []error { return []error{assert.AnError} },
			},
			rv1:     &bundle.RegistryV1{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.v.Validate(tt.rv1)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResourceGenerators_GenerateResources(t *testing.T) {
	opts := defaultTestOpts()

	tests := []struct {
		name    string
		gens    ResourceGenerators
		rv1     *bundle.RegistryV1
		wantErr bool
		wantLen int
	}{
		{
			name: "no generators",
			gens: ResourceGenerators{},
			rv1:  &bundle.RegistryV1{},
		},
		{
			name: "single generator",
			gens: ResourceGenerators{
				func(_ *bundle.RegistryV1, _ Options) ([]client.Object, error) {
					return []client.Object{
						CreateServiceAccountResource("test-sa", "test-ns"),
					}, nil
				},
			},
			rv1:     &bundle.RegistryV1{},
			wantLen: 1,
		},
		{
			name: "error propagation",
			gens: ResourceGenerators{
				func(_ *bundle.RegistryV1, _ Options) ([]client.Object, error) {
					return nil, assert.AnError
				},
			},
			rv1:     &bundle.RegistryV1{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := tt.gens.GenerateResources(tt.rv1, opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, objs, tt.wantLen)
		})
	}
}

func TestBundleConverter_Convert(t *testing.T) {
	tests := []struct {
		name    string
		bc      BundleConverter
		rv1     bundle.RegistryV1
		ns      string
		opts    []Option
		wantErr bool
		wantLen int
	}{
		{
			name: "minimal valid bundle succeeds",
			bc: BundleConverter{
				BundleValidator: BundleValidator{},
				ResourceGenerators: []ResourceGenerator{
					func(_ *bundle.RegistryV1, _ Options) ([]client.Object, error) {
						return []client.Object{
							CreateServiceAccountResource("sa", "ns"),
						}, nil
					},
				},
			},
			rv1: bundle.RegistryV1{
				PackageName: "test",
				CSV: v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "test-csv"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallModes: []v1alpha1.InstallMode{
							{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
						},
						InstallStrategy: v1alpha1.NamedInstallStrategy{
							StrategySpec: v1alpha1.StrategyDetailsDeployment{
								DeploymentSpecs: []v1alpha1.StrategyDeploymentSpec{
									{
										Name: "deploy",
										Spec: appsv1.DeploymentSpec{
											Template: corev1.PodTemplateSpec{},
										},
									},
								},
							},
						},
					},
				},
			},
			ns:      "operator-ns",
			wantLen: 1,
		},
		{
			name: "validation fails",
			bc: BundleConverter{
				BundleValidator: BundleValidator{
					func(_ *bundle.RegistryV1) []error { return []error{assert.AnError} },
				},
			},
			rv1:     bundle.RegistryV1{},
			ns:      "operator-ns",
			wantErr: true,
		},
		{
			name: "invalid target namespaces",
			bc: BundleConverter{
				BundleValidator: BundleValidator{},
			},
			rv1: bundle.RegistryV1{
				PackageName: "test",
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallModes: []v1alpha1.InstallMode{
							{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
						},
					},
				},
			},
			ns:      "operator-ns",
			opts:    []Option{WithTargetNamespaces("ns1", "ns2")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := tt.bc.Convert(tt.rv1, tt.ns, tt.opts...)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, objs, tt.wantLen)
		})
	}
}

func TestValidateTargetNamespaces(t *testing.T) {
	tests := []struct {
		name             string
		installModes     []v1alpha1.InstallMode
		installNamespace string
		targetNamespaces []string
		wantErr          bool
	}{
		{
			name: "empty with MultiNamespace supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeMultiNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{},
			wantErr:          true,
		},
		{
			name: "empty with no MultiNamespace",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{},
			wantErr:          true,
		},
		{
			name: "AllNamespaces with empty string supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{""},
			wantErr:          false,
		},
		{
			name: "AllNamespaces with empty string not supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{""},
			wantErr:          true,
		},
		{
			name: "own namespace supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"operator-ns"},
			wantErr:          false,
		},
		{
			name: "own namespace not supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeSingleNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"operator-ns"},
			wantErr:          true,
		},
		{
			name: "single other namespace supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeSingleNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"other-ns"},
			wantErr:          false,
		},
		{
			name: "single other namespace not supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"other-ns"},
			wantErr:          true,
		},
		{
			name: "multi namespace supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeMultiNamespace, Supported: true},
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"ns1", "ns2", "operator-ns"},
			wantErr:          false,
		},
		{
			name: "multi namespace without own namespace support",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeMultiNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"ns1", "operator-ns"},
			wantErr:          true,
		},
		{
			name: "multi namespace not supported",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"ns1", "ns2"},
			wantErr:          true,
		},
		{
			name: "multi namespace with empty string fails",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeMultiNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"ns1", ""},
			wantErr:          true,
		},
		{
			name: "multi namespace without own namespace in list",
			installModes: []v1alpha1.InstallMode{
				{Type: v1alpha1.InstallModeTypeMultiNamespace, Supported: true},
			},
			installNamespace: "operator-ns",
			targetNamespaces: []string{"ns1", "ns2"},
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv1 := &bundle.RegistryV1{
				CSV: v1alpha1.ClusterServiceVersion{
					Spec: v1alpha1.ClusterServiceVersionSpec{
						InstallModes: tt.installModes,
					},
				},
			}
			err := validateTargetNamespaces(rv1, tt.installNamespace, tt.targetNamespaces)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultUniqueNameGenerator(t *testing.T) {
	name := DefaultUniqueNameGenerator("my-base", struct{ Field string }{Field: "value"})

	// Verify format is "base-hash"
	assert.True(t, strings.HasPrefix(name, "my-base-"), "expected name to start with 'my-base-', got %q", name)

	// Verify length <= 63
	assert.LessOrEqual(t, len(name), 63)

	// Verify determinism
	name2 := DefaultUniqueNameGenerator("my-base", struct{ Field string }{Field: "value"})
	assert.Equal(t, name, name2)
}

func TestWithTargetNamespaces(t *testing.T) {
	opts := &Options{}
	fn := WithTargetNamespaces("ns1", "ns2")
	fn(opts)
	assert.Equal(t, []string{"ns1", "ns2"}, opts.TargetNamespaces)
}

func TestWithCertificateProvider(t *testing.T) {
	provider := &fakeCertProvider{}
	opts := &Options{}
	fn := WithCertificateProvider(provider)
	fn(opts)
	assert.Equal(t, provider, opts.CertificateProvider)
}
