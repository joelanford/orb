package ref

import (
	"fmt"
	"strings"

	"github.com/joelanford/orb/internal/format"
	"github.com/joelanford/orb/internal/transport"
)

// Parsed holds a fully parsed "format:transport:ref" argument.
type Parsed struct {
	Format    format.Format
	Transport transport.TransportRef
}

// Parse splits a "format:transport:ref" string into its components.
// The first colon separates the format prefix; the remainder is a transport:ref string.
// Special case: "plain:stdout" is handled as format=plain, transport=stdout.
func Parse(s string) (Parsed, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return Parsed{}, fmt.Errorf("invalid argument %q: expected format:transport:ref", s)
	}

	formatStr := s[:idx]
	remainder := s[idx+1:]

	f, err := format.ParseFormat(formatStr)
	if err != nil {
		return Parsed{}, err
	}

	tr, err := transport.ParseTransportRef(remainder)
	if err != nil {
		return Parsed{}, err
	}

	return Parsed{Format: f, Transport: tr}, nil
}

// ValidateSource checks that the parsed ref is valid as a source argument.
func ValidateSource(p Parsed) error {
	if !format.CanBeSource(p.Format) {
		return fmt.Errorf("%s cannot be used as a source", p.Format)
	}
	if p.Transport.Transport == transport.Stdout {
		return fmt.Errorf("stdout cannot be used as a source transport")
	}
	return nil
}

// ValidateDestination checks that the parsed ref is valid as a destination argument.
func ValidateDestination(p Parsed) error {
	if !format.CanBeDestination(p.Format) {
		return fmt.Errorf("%s cannot be used as a destination", p.Format)
	}
	if p.Format == format.Plain {
		switch p.Transport.Transport {
		case transport.Dir, transport.Stdout:
			// allowed
		default:
			return fmt.Errorf("%s transport not supported for plain format", p.Transport.Transport)
		}
	}
	return nil
}
