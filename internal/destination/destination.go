package destination

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/format"
	"github.com/joelanford/orb/internal/ref"
	"github.com/joelanford/orb/internal/transport"
)

// Destination writes a bundle to a destination location.
type Destination interface {
	Write(ctx context.Context, b *bundle.Bundle) error
}

// Options holds authentication and TLS settings for destination transports.
type Options struct {
	Username  string
	Password  string
	TLSVerify bool
	CertDir   string
	NoCreds   bool
}

// New creates a Destination for the given parsed reference and options.
func New(p ref.Parsed, opts Options) (Destination, error) {
	switch p.Format {
	case format.Helm:
		return newHelm(p.Transport, opts)
	case format.Plain:
		return newPlain(p.Transport, opts)
	default:
		return nil, fmt.Errorf("unsupported destination format: %s", p.Format)
	}
}

func newHelm(tr transport.TransportRef, opts Options) (Destination, error) {
	switch tr.Transport {
	case transport.Docker:
		return &helmDocker{ref: tr.Ref, opts: opts}, nil
	case transport.OCI:
		return &helmOCI{ref: tr.Ref, opts: opts}, nil
	case transport.OCIArchive:
		return &helmOCIArchive{ref: tr.Ref, opts: opts}, nil
	case transport.Dir:
		return &helmDir{ref: tr.Ref, opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported helm transport: %s", tr.Transport)
	}
}

func newPlain(tr transport.TransportRef, opts Options) (Destination, error) {
	switch tr.Transport {
	case transport.Dir:
		return &plainDir{ref: tr.Ref, opts: opts}, nil
	case transport.Stdout:
		return &plainStdout{opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported plain transport: %s", tr.Transport)
	}
}
