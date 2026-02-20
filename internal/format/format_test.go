package format

import (
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    Format
		wantErr bool
	}{
		{"regv1", RegV1, false},
		{"helm", Helm, false},
		{"plain", Plain, false},
		{"foo", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFormat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		f    Format
		want string
	}{
		{RegV1, "regv1"},
		{Helm, "helm"},
		{Plain, "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Errorf("Format.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanBeSource(t *testing.T) {
	tests := []struct {
		f    Format
		want bool
	}{
		{RegV1, true},
		{Helm, false},
		{Plain, false},
	}
	for _, tt := range tests {
		t.Run(tt.f.String(), func(t *testing.T) {
			if got := CanBeSource(tt.f); got != tt.want {
				t.Errorf("CanBeSource(%v) = %v, want %v", tt.f, got, tt.want)
			}
		})
	}
}

func TestCanBeDestination(t *testing.T) {
	tests := []struct {
		f    Format
		want bool
	}{
		{RegV1, false},
		{Helm, true},
		{Plain, true},
	}
	for _, tt := range tests {
		t.Run(tt.f.String(), func(t *testing.T) {
			if got := CanBeDestination(tt.f); got != tt.want {
				t.Errorf("CanBeDestination(%v) = %v, want %v", tt.f, got, tt.want)
			}
		})
	}
}
