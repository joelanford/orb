package destination

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
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

func (d *helmDir) Write(_ context.Context, _ *bundle.RegistryV1) error {
	return fmt.Errorf("helm dir destination not yet implemented (ref: %s)", d.ref)
}
