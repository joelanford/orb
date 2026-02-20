package destination

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
)

type plainDir struct {
	ref  string
	opts Options
}

func (d *plainDir) Write(_ context.Context, _ *bundle.Bundle) error {
	return fmt.Errorf("plain dir destination not yet implemented (ref: %s)", d.ref)
}

type plainStdout struct {
	opts Options
}

func (d *plainStdout) Write(_ context.Context, _ *bundle.Bundle) error {
	return fmt.Errorf("plain stdout destination not yet implemented")
}
