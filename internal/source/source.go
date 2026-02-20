package source

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/ref"
	"github.com/joelanford/orb/internal/transport"
)

// Source reads a bundle from a source location.
type Source interface {
	Read(ctx context.Context) (*bundle.Bundle, error)
}

// Options holds authentication and TLS settings for source transports.
type Options struct {
	Username  string
	Password  string
	TLSVerify bool
	CertDir   string
	NoCreds   bool
}

// New creates a Source for the given parsed reference and options.
func New(p ref.Parsed, opts Options) (Source, error) {
	switch p.Transport.Transport {
	case transport.Docker:
		return &regv1Docker{ref: p.Transport.Ref, opts: opts}, nil
	case transport.OCI:
		return &regv1OCI{ref: p.Transport.Ref, opts: opts}, nil
	case transport.OCIArchive:
		return &regv1OCIArchive{ref: p.Transport.Ref, opts: opts}, nil
	case transport.Dir:
		return &regv1Dir{ref: p.Transport.Ref, opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported source transport: %s", p.Transport.Transport)
	}
}
