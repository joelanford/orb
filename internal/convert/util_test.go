package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectNameForBaseAndSuffix(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		suffix string
		want   string
	}{
		{
			name:   "short names",
			base:   "myoperator",
			suffix: "abc",
			want:   "myoperator-abc",
		},
		{
			name:   "exact max length",
			base:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 58 chars
			suffix: "abcd",                                                       // 4 chars => 58 + 4 = 62, not > 63, no truncation => output 58+1+4=63
			want:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-abcd",
		},
		{
			name:   "exceeds max length truncated",
			base:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 63 chars
			suffix: "abcd",                                                            // 63 + 4 = 67 > 63, truncated => base becomes 58 chars
			want:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-abcd", //nolint:gocritic // 58+1+4=63
		},
		{
			name:   "empty suffix",
			base:   "myoperator",
			suffix: "",
			want:   "myoperator-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ObjectNameForBaseAndSuffix(tc.base, tc.suffix)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, len(got), 63)
		})
	}
}

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name string
		maps []map[string]string
		want map[string]string
	}{
		{
			name: "empty",
			maps: nil,
			want: map[string]string{},
		},
		{
			name: "single map",
			maps: []map[string]string{{"a": "1", "b": "2"}},
			want: map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "overlapping keys",
			maps: []map[string]string{
				{"a": "1", "b": "2"},
				{"b": "3", "c": "4"},
			},
			want: map[string]string{"a": "1", "b": "3", "c": "4"},
		},
		{
			name: "nil maps",
			maps: []map[string]string{nil, {"a": "1"}, nil},
			want: map[string]string{"a": "1"},
		},
		{
			name: "disjoint maps",
			maps: []map[string]string{
				{"a": "1"},
				{"b": "2"},
				{"c": "3"},
			},
			want: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeMaps(tc.maps...)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDeepHashObject(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		obj := map[string]string{"key": "value"}
		h1 := DeepHashObject(obj)
		h2 := DeepHashObject(obj)
		assert.Equal(t, h1, h2)
		assert.NotEmpty(t, h1)
	})

	t.Run("different objects differ", func(t *testing.T) {
		obj1 := map[string]string{"key": "value1"}
		obj2 := map[string]string{"key": "value2"}
		h1 := DeepHashObject(obj1)
		h2 := DeepHashObject(obj2)
		assert.NotEqual(t, h1, h2)
	})
}
