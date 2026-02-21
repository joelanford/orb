package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestToUnstructured(t *testing.T) {
	tests := []struct {
		name    string
		obj     func() *corev1.ConfigMap
		wantErr string
	}{
		{
			name: "valid typed object",
			obj: func() *corev1.ConfigMap {
				cm := &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ConfigMap",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cm",
						Namespace: "default",
					},
					Data: map[string]string{"key": "value"},
				}
				return cm
			},
		},
		{
			name: "missing kind",
			obj: func() *corev1.ConfigMap {
				cm := &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cm",
					},
				}
				return cm
			},
			wantErr: "object has no kind",
		},
		{
			name: "missing version",
			obj: func() *corev1.ConfigMap {
				cm := &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						Kind: "ConfigMap",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cm",
					},
				}
				return cm
			},
			wantErr: "object has no version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := ToUnstructured(tc.obj())
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, u)
			} else {
				require.NoError(t, err)
				require.NotNil(t, u)
				assert.Equal(t, "test-cm", u.GetName())
			}
		})
	}

	t.Run("nil object", func(t *testing.T) {
		u, err := ToUnstructured(nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, "object is nil")
		assert.Nil(t, u)
	})
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
