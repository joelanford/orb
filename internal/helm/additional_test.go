package helm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

// renderAdditional generates a helm chart from the bundle, renders the
// additional.yaml template with the given values, and parses each document
// into an unstructured.Unstructured slice.
func renderAdditional(t *testing.T, b *bundle.RegistryV1) []unstructured.Unstructured {
	t.Helper()

	rendered, err := renderChart(t, b, map[string]any{"watchNamespace": ""})
	require.NoError(t, err)

	var addYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "additional.yaml") {
			addYAML = data
			break
		}
	}
	require.NotEmpty(t, addYAML, "additional.yaml not found in rendered output")

	var objs []unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(addYAML), 4096)
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			break
		}
		if obj.GetKind() != "" || obj.GetName() != "" {
			objs = append(objs, obj)
		}
	}
	require.NotEmpty(t, objs, "failed to parse any objects from rendered additional.yaml:\n%s", addYAML)
	return objs
}

func TestAdditional_NamespacedResource(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.Others = []unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "original-ns",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
		}
	})

	objs := renderAdditional(t, b)
	require.Len(t, objs, 1)

	obj := objs[0]
	assert.Equal(t, "ConfigMap", obj.GetKind())
	assert.Equal(t, "test-config", obj.GetName())
	assert.Equal(t, "test-ns", obj.GetNamespace(), "namespace should be set to the release namespace")
}

func TestAdditional_UnsupportedKind(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.Others = []unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
				},
			},
		}
	})

	_, err := generateAdditional(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resource")
}

func TestAdditional_Empty(t *testing.T) {
	b := makeMinimalBundle()

	c, err := Generate(b)
	require.NoError(t, err)

	// Verify that additional.yaml is not present in the chart templates.
	for _, tmpl := range c.Templates {
		assert.False(t, strings.HasSuffix(tmpl.Name, "additional.yaml"),
			"additional.yaml should not be present when there are no additional resources")
	}
}
