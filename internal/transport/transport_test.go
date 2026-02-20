package transport

import (
	"testing"
)

func TestParseTransportRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTr  Transport
		wantRef string
		wantErr bool
	}{
		{"docker", "docker://quay.io/my/bundle:v1", Docker, "quay.io/my/bundle:v1", false},
		{"oci", "oci:/path/to/layout:latest", OCI, "/path/to/layout:latest", false},
		{"oci-archive", "oci-archive:/path/to/chart.tar", OCIArchive, "/path/to/chart.tar", false},
		{"dir", "dir:/path/to/bundle", Dir, "/path/to/bundle", false},
		{"tar", "tar:/path/to/bundle.tar.gz", Tar, "/path/to/bundle.tar.gz", false},
		{"tar uncompressed", "tar:bundle.tar", Tar, "bundle.tar", false},
		{"stdout", "stdout", Stdout, "", false},
		{"unknown", "foo:bar", 0, "", true},
		{"empty", "", 0, "", true},
		{"oci-archive before oci", "oci-archive:/tmp/test", OCIArchive, "/tmp/test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTransportRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTransportRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Transport != tt.wantTr {
				t.Errorf("ParseTransportRef(%q).Transport = %v, want %v", tt.input, got.Transport, tt.wantTr)
			}
			if got.Ref != tt.wantRef {
				t.Errorf("ParseTransportRef(%q).Ref = %q, want %q", tt.input, got.Ref, tt.wantRef)
			}
		})
	}
}

func TestTransportString(t *testing.T) {
	tests := []struct {
		tr   Transport
		want string
	}{
		{Docker, "docker://"},
		{OCI, "oci:"},
		{OCIArchive, "oci-archive:"},
		{Dir, "dir:"},
		{Tar, "tar:"},
		{Stdout, "stdout"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tr.String(); got != tt.want {
				t.Errorf("Transport.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
