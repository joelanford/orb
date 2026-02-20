package source

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
)

type regv1Docker struct {
	ref  string
	opts Options
}

func (s *regv1Docker) Read(_ context.Context) (*bundle.Bundle, error) {
	return nil, fmt.Errorf("regv1 docker source not yet implemented (ref: %s)", s.ref)
}

type regv1OCI struct {
	ref  string
	opts Options
}

func (s *regv1OCI) Read(_ context.Context) (*bundle.Bundle, error) {
	return nil, fmt.Errorf("regv1 oci source not yet implemented (ref: %s)", s.ref)
}

type regv1OCIArchive struct {
	ref  string
	opts Options
}

func (s *regv1OCIArchive) Read(_ context.Context) (*bundle.Bundle, error) {
	return nil, fmt.Errorf("regv1 oci-archive source not yet implemented (ref: %s)", s.ref)
}

type regv1Dir struct {
	ref  string
	opts Options
}

func (s *regv1Dir) Read(_ context.Context) (*bundle.Bundle, error) {
	return nil, fmt.Errorf("regv1 dir source not yet implemented (ref: %s)", s.ref)
}
