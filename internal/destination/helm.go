package destination

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	chartFiles, err := helm.Generate(b)
	if err != nil {
		return fmt.Errorf("generating helm chart: %w", err)
	}

	dir := expandPath(d.ref)

	for relPath, content := range chartFiles {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
	}

	return nil
}
