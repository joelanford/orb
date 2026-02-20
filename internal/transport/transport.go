package transport

import (
	"fmt"
	"strings"
)

type Transport int

const (
	Docker Transport = iota
	OCI
	OCIArchive
	Dir
	Stdout
)

func (t Transport) String() string {
	switch t {
	case Docker:
		return "docker://"
	case OCI:
		return "oci:"
	case OCIArchive:
		return "oci-archive:"
	case Dir:
		return "dir:"
	case Stdout:
		return "stdout"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// TransportRef holds a parsed transport and its reference string.
type TransportRef struct {
	Transport Transport
	Ref       string
}

// prefixes is ordered longest-first so that "oci-archive:" matches before "oci:".
var prefixes = []struct {
	prefix    string
	transport Transport
}{
	{"docker://", Docker},
	{"oci-archive:", OCIArchive},
	{"oci:", OCI},
	{"dir:", Dir},
}

// ParseTransportRef parses a "transport:ref" string using longest-prefix-first matching.
func ParseTransportRef(s string) (TransportRef, error) {
	if s == "stdout" {
		return TransportRef{Transport: Stdout}, nil
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p.prefix) {
			return TransportRef{
				Transport: p.transport,
				Ref:       s[len(p.prefix):],
			}, nil
		}
	}
	return TransportRef{}, fmt.Errorf("unknown transport in %q", s)
}
