package destination

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/helm"
)

type helmDocker struct {
	ref  string
	opts Options
}

func (d *helmDocker) Write(_ context.Context, _ *bundle.RegistryV1) error {
	return fmt.Errorf("helm docker destination not yet implemented (ref: %s)", d.ref)
}

type helmOCI struct {
	ref  string
	opts Options
}

func (d *helmOCI) Write(_ context.Context, _ *bundle.RegistryV1) error {
	return fmt.Errorf("helm oci destination not yet implemented (ref: %s)", d.ref)
}

type helmOCIArchive struct {
	ref  string
	opts Options
}

func (d *helmOCIArchive) Write(_ context.Context, _ *bundle.RegistryV1) error {
	return fmt.Errorf("helm oci-archive destination not yet implemented (ref: %s)", d.ref)
}

type helmDir struct {
	ref  string
	opts Options
}

func (d *helmDir) Write(_ context.Context, b *bundle.RegistryV1) error {
	c, err := helm.Generate(b)
	if err != nil {
		return fmt.Errorf("generating helm chart: %w", err)
	}
	return chartutil.SaveDir(c, expandPath(d.ref))
}

type helmChartArchive struct {
	ref  string
	opts Options
}

func (d *helmChartArchive) Write(_ context.Context, b *bundle.RegistryV1) error {
	c, err := helm.Generate(b)
	if err != nil {
		return fmt.Errorf("generating helm chart: %w", err)
	}

	ref := expandPath(d.ref)

	if strings.HasSuffix(ref, ".tgz") {
		dir := filepath.Dir(ref)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
		// Use a temp dir under the same parent to ensure same-filesystem rename.
		tmpDir, err := os.MkdirTemp(dir, ".helm-chart-archive-*")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		archivePath, err := chartutil.Save(c, tmpDir)
		if err != nil {
			return fmt.Errorf("saving chart archive: %w", err)
		}
		return os.Rename(archivePath, ref)
	}

	// Treat ref as a directory.
	if err := os.MkdirAll(ref, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	_, err = chartutil.Save(c, ref)
	return err
}
