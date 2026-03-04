package destination

import (
	"context"
	"fmt"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
	"github.com/joelanford/orb/internal/transport"
)

// Destination writes a bundle to a destination location.
type Destination interface {
	Write(ctx context.Context, b *bundle.RegistryV1) error
}

// Options holds authentication, TLS, and render settings for destination transports.
type Options struct {
	Namespace   string
	ConvertOpts []convert.Option
}

func NewHelm(tr transport.Ref, opts Options) (Destination, error) {
	switch tr.Transport {
	case transport.Docker:
		return &helmDocker{ref: tr.Ref, opts: opts}, nil
	case transport.OCI:
		return &helmOCI{ref: tr.Ref, opts: opts}, nil
	case transport.OCIArchive:
		return &helmOCIArchive{ref: tr.Ref, opts: opts}, nil
	case transport.Dir:
		return &helmDir{ref: tr.Ref, opts: opts}, nil
	case transport.ChartArchive:
		return &helmChartArchive{ref: tr.Ref, opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported helm transport: %s", tr.Transport)
	}
}

func NewPlain(tr transport.Ref, opts Options) (Destination, error) {
	switch tr.Transport {
	case transport.Dir:
		return &plainDir{ref: tr.Ref, opts: opts}, nil
	case transport.Stdout:
		return &plainStdout{opts: opts}, nil
	default:
		return nil, fmt.Errorf("unsupported plain transport: %s", tr.Transport)
	}
}
