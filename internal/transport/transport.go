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
	Tar
	Stdout
	ChartArchive
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
	case Tar:
		return "tar:"
	case Stdout:
		return "stdout"
	case ChartArchive:
		return "chart-archive:"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// Ref holds a parsed transport and its reference string.
type Ref struct {
	Transport Transport
	Ref       string
}

// prefixes is ordered longest-first so that "oci-archive:" matches before "oci:".
var prefixes = []struct {
	prefix    string
	transport Transport
}{
	{"docker://", Docker},
	{"chart-archive:", ChartArchive},
	{"oci-archive:", OCIArchive},
	{"oci:", OCI},
	{"tar:", Tar},
	{"dir:", Dir},
}

// ParseRef parses a "transport:ref" string using longest-prefix-first matching.
func ParseRef(s string) (Ref, error) {
	if s == "stdout" {
		return Ref{Transport: Stdout}, nil
	}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p.prefix) {
			return Ref{
				Transport: p.transport,
				Ref:       s[len(p.prefix):],
			}, nil
		}
	}
	return Ref{}, fmt.Errorf("unknown transport in %q", s)
}
