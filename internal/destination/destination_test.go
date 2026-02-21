package destination

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/transport"
)

func TestNewHelm(t *testing.T) {
	tests := []struct {
		name      string
		transport transport.Transport
		wantErr   bool
	}{
		{"Docker", transport.Docker, false},
		{"OCI", transport.OCI, false},
		{"OCIArchive", transport.OCIArchive, false},
		{"Dir", transport.Dir, false},
		{"Unsupported", transport.Stdout, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest, err := NewHelm(transport.Ref{Transport: tt.transport, Ref: "test-ref"}, Options{})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported")
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, dest)
		})
	}
}

func TestNewPlain(t *testing.T) {
	tests := []struct {
		name      string
		transport transport.Transport
		wantErr   bool
	}{
		{"Dir", transport.Dir, false},
		{"Stdout", transport.Stdout, false},
		{"Unsupported", transport.Docker, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest, err := NewPlain(transport.Ref{Transport: tt.transport, Ref: "test-ref"}, Options{})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported")
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, dest)
		})
	}
}

func TestHelmDocker_Write_NotImplemented(t *testing.T) {
	d := &helmDocker{ref: "example.com/chart:v1"}
	err := d.Write(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestHelmOCI_Write_NotImplemented(t *testing.T) {
	d := &helmOCI{ref: "example.com/chart:v1"}
	err := d.Write(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestHelmOCIArchive_Write_NotImplemented(t *testing.T) {
	d := &helmOCIArchive{ref: "example.com/chart:v1"}
	err := d.Write(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestPlainDir_Write(t *testing.T) {
	dir := t.TempDir()
	b := minimalBundle()
	d := &plainDir{
		ref: dir,
		opts: Options{
			Namespace: "test-ns",
		},
	}

	err := d.Write(context.Background(), b)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "manifests.yaml"))
	require.NoError(t, err)
}

func TestHelmDir_Write(t *testing.T) {
	dir := t.TempDir()
	b := minimalBundle()
	d := &helmDir{
		ref: dir,
	}

	err := d.Write(context.Background(), b)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, b.PackageName, "Chart.yaml"))
	require.NoError(t, err)
}

// minimalBundle creates a minimal valid bundle.RegistryV1 that passes validation.
func minimalBundle() *bundle.RegistryV1 {
	return &bundle.RegistryV1{
		PackageName: "test-operator",
		CSV: v1alpha1.ClusterServiceVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-operator.v0.1.0",
			},
			Spec: v1alpha1.ClusterServiceVersionSpec{
				InstallModes: []v1alpha1.InstallMode{
					{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
				},
				InstallStrategy: v1alpha1.NamedInstallStrategy{
					StrategyName: "deployment",
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
													Image: "example.com/operator:v0.1.0",
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
}
