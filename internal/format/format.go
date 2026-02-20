package format

import "fmt"

type Format int

const (
	RegV1 Format = iota
	Helm
	Plain
)

func (f Format) String() string {
	switch f {
	case RegV1:
		return "regv1"
	case Helm:
		return "helm"
	case Plain:
		return "plain"
	default:
		return fmt.Sprintf("unknown(%d)", int(f))
	}
}

func ParseFormat(s string) (Format, error) {
	switch s {
	case "regv1":
		return RegV1, nil
	case "helm":
		return Helm, nil
	case "plain":
		return Plain, nil
	default:
		return 0, fmt.Errorf("unknown format %q", s)
	}
}

func CanBeSource(f Format) bool {
	return f == RegV1
}

func CanBeDestination(f Format) bool {
	return f == Helm || f == Plain
}
