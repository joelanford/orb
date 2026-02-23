package destination

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	bsemver "github.com/blang/semver/v4"
	opversion "github.com/operator-framework/api/pkg/lib/version"
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
		{"ChartArchive", transport.ChartArchive, false},
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

func TestPlainDir_Write_FailsIfExists(t *testing.T) {
	dir := t.TempDir()
	d := &plainDir{
		ref:  dir,
		opts: Options{Namespace: "test-ns"},
	}

	err := d.Write(context.Background(), minimalBundle())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestPlainDir_Write(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
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

func TestHelmDir_Write_FailsIfExists(t *testing.T) {
	dir := t.TempDir()
	d := &helmDir{ref: dir}

	err := d.Write(context.Background(), minimalBundle())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestHelmDir_Write(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	b := minimalBundle()
	d := &helmDir{
		ref: dir,
	}

	err := d.Write(context.Background(), b)
	require.NoError(t, err)

	// Chart.yaml should be directly in the output dir, not in a subdirectory.
	_, err = os.Stat(filepath.Join(dir, "Chart.yaml"))
	require.NoError(t, err)
}

func TestHelmChartArchive_Write_FailsIfTgzExists(t *testing.T) {
	tgzPath := filepath.Join(t.TempDir(), "my-chart.tgz")
	require.NoError(t, os.WriteFile(tgzPath, []byte("existing"), 0o644))
	d := &helmChartArchive{ref: tgzPath}

	err := d.Write(context.Background(), minimalBundleWithVersion())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestHelmChartArchive_Write_FailsIfDirExists(t *testing.T) {
	dir := t.TempDir()
	d := &helmChartArchive{ref: dir}

	err := d.Write(context.Background(), minimalBundleWithVersion())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestHelmChartArchive_Write_TgzPath(t *testing.T) {
	dir := t.TempDir()
	tgzPath := filepath.Join(dir, "my-chart.tgz")
	b := minimalBundleWithVersion()
	d := &helmChartArchive{ref: tgzPath}

	err := d.Write(context.Background(), b)
	require.NoError(t, err)

	// File should exist at the exact path
	_, err = os.Stat(tgzPath)
	require.NoError(t, err)

	// Verify it is a valid gzip/tar archive containing chart files
	assertValidChartArchive(t, tgzPath, b.PackageName)
}

func TestHelmChartArchive_Write_DirPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	b := minimalBundleWithVersion()
	d := &helmChartArchive{ref: dir}

	err := d.Write(context.Background(), b)
	require.NoError(t, err)

	// Save creates <name>-<version>.tgz in the directory
	matches, err := filepath.Glob(filepath.Join(dir, "*.tgz"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	assertValidChartArchive(t, matches[0], b.PackageName)
}

func assertValidChartArchive(t *testing.T, path, chartName string) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer gz.Close()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, hdr.Name)
	}

	assert.NotEmpty(t, names)
	// Chart.yaml must be present under the chart name directory
	var foundChart bool
	for _, n := range names {
		if n == chartName+"/Chart.yaml" {
			foundChart = true
			break
		}
	}
	assert.True(t, foundChart, "expected %s/Chart.yaml in archive, got: %v", chartName, names)
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

// minimalBundleWithVersion returns a bundle with a proper semver version for chart archive tests.
func minimalBundleWithVersion() *bundle.RegistryV1 {
	b := minimalBundle()
	b.CSV.Spec.Version = opversion.OperatorVersion{Version: bsemver.MustParse("0.1.0")}
	return b
}
