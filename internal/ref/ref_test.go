package ref

import (
	"strings"
	"testing"

	"github.com/joelanford/orb/internal/format"
	"github.com/joelanford/orb/internal/transport"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFormat  format.Format
		wantTr      transport.Transport
		wantRef     string
		wantErr     bool
		errContains string
	}{
		{"regv1 docker", "regv1:docker://quay.io/my/bundle:v1", format.RegV1, transport.Docker, "quay.io/my/bundle:v1", false, ""},
		{"regv1 oci", "regv1:oci:/path/to/layout:latest", format.RegV1, transport.OCI, "/path/to/layout:latest", false, ""},
		{"regv1 oci-archive", "regv1:oci-archive:/path/to/bundle.tar", format.RegV1, transport.OCIArchive, "/path/to/bundle.tar", false, ""},
		{"regv1 dir", "regv1:dir:/path/to/bundle", format.RegV1, transport.Dir, "/path/to/bundle", false, ""},
		{"helm docker", "helm:docker://quay.io/my/chart:v1", format.Helm, transport.Docker, "quay.io/my/chart:v1", false, ""},
		{"helm oci", "helm:oci:/path/to/layout:latest", format.Helm, transport.OCI, "/path/to/layout:latest", false, ""},
		{"helm oci-archive", "helm:oci-archive:/path/to/chart.tar", format.Helm, transport.OCIArchive, "/path/to/chart.tar", false, ""},
		{"helm dir", "helm:dir:/tmp/chart", format.Helm, transport.Dir, "/tmp/chart", false, ""},
		{"plain dir", "plain:dir:/tmp/manifests", format.Plain, transport.Dir, "/tmp/manifests", false, ""},
		{"plain stdout", "plain:stdout", format.Plain, transport.Stdout, "", false, ""},
		{"unknown format", "foo:dir:/tmp", 0, 0, "", true, "unknown format"},
		{"no colon", "regv1", 0, 0, "", true, "expected format:transport:ref"},
		{"unknown transport", "regv1:bogus:/tmp", 0, 0, "", true, "unknown transport"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Parse(%q) error = %v, want error containing %q", tt.input, err, tt.errContains)
				}
				return
			}
			if got.Format != tt.wantFormat {
				t.Errorf("Parse(%q).Format = %v, want %v", tt.input, got.Format, tt.wantFormat)
			}
			if got.Transport.Transport != tt.wantTr {
				t.Errorf("Parse(%q).Transport = %v, want %v", tt.input, got.Transport.Transport, tt.wantTr)
			}
			if got.Transport.Ref != tt.wantRef {
				t.Errorf("Parse(%q).Ref = %q, want %q", tt.input, got.Transport.Ref, tt.wantRef)
			}
		})
	}
}

func TestValidateSource(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		{"regv1 docker", "regv1:docker://quay.io/my/bundle:v1", false, ""},
		{"regv1 dir", "regv1:dir:/path/to/bundle", false, ""},
		{"regv1 oci", "regv1:oci:/path/to/layout:latest", false, ""},
		{"regv1 oci-archive", "regv1:oci-archive:/path/to/bundle.tar", false, ""},
		{"helm not source", "helm:dir:/tmp/chart", true, "cannot be used as a source"},
		{"plain not source", "plain:dir:/tmp", true, "cannot be used as a source"},
		{"regv1 stdout", "regv1:stdout", true, "stdout cannot be used as a source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			err = ValidateSource(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSource(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("ValidateSource(%q) error = %v, want containing %q", tt.input, err, tt.errContains)
			}
		})
	}
}

func TestValidateDestination(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		{"helm docker", "helm:docker://quay.io/my/chart:v1", false, ""},
		{"helm dir", "helm:dir:/tmp/chart", false, ""},
		{"helm oci", "helm:oci:/path/to/layout:latest", false, ""},
		{"helm oci-archive", "helm:oci-archive:/path/to/chart.tar", false, ""},
		{"plain dir", "plain:dir:/tmp/manifests", false, ""},
		{"plain stdout", "plain:stdout", false, ""},
		{"regv1 not dest", "regv1:dir:/tmp/bundle", true, "cannot be used as a destination"},
		{"plain docker", "plain:docker://quay.io/foo:v1", true, "not supported for plain"},
		{"plain oci", "plain:oci:/path/to/layout:latest", true, "not supported for plain"},
		{"plain oci-archive", "plain:oci-archive:/path/to/chart.tar", true, "not supported for plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			err = ValidateDestination(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDestination(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("ValidateDestination(%q) error = %v, want containing %q", tt.input, err, tt.errContains)
			}
		})
	}
}
