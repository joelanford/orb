package source

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/transport"
)

// Source reads a bundle from a source location.
type Source interface {
	Read(ctx context.Context) (*bundle.RegistryV1, error)
}

// Options holds authentication and TLS settings for source transports.
type Options struct {
	Username  string
	Password  string
	TLSVerify bool
	CertDir   string
	NoCreds   bool
}

// New creates a Source for the given transport reference and options.
func New(tr transport.TransportRef, opts Options) (Source, error) {
	switch tr.Transport {
	case transport.Docker:
		return &regv1Docker{ref: tr.Ref, opts: opts}, nil
	case transport.OCI:
		return &regv1OCI{ref: tr.Ref, opts: opts}, nil
	case transport.OCIArchive:
		return &regv1OCIArchive{ref: tr.Ref, opts: opts}, nil
	case transport.Dir:
		return &regv1Dir{ref: tr.Ref, opts: opts}, nil
	case transport.Tar:
		return &regv1Tar{ref: tr.Ref, opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported source transport: %s", tr.Transport)
	}
}
