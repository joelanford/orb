package helm

import (
	"encoding/json"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestGenerateSchema_AllNamespacesOnly(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeAllNamespaces)
	data, err := generateSchema(b, modes, false)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	wnSchema := props["watchNamespace"].(map[string]interface{})
	assert.Equal(t, "", wnSchema["const"])
}

func TestGenerateSchema_NonAllNamespacesOnly(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeSingleNamespace)
	data, err := generateSchema(b, modes, false)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	wnSchema := props["watchNamespace"].(map[string]interface{})
	assert.Equal(t, "string", wnSchema["type"])
	assert.Equal(t, float64(1), wnSchema["minLength"])
	assert.Equal(t, float64(namespaceNameMaxLength), wnSchema["maxLength"])
	assert.Equal(t, namespaceNamePattern, wnSchema["pattern"])

	// watchNamespace should be required
	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "watchNamespace")
}

func TestGenerateSchema_BothNamespaceModes(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](
		v1alpha1.InstallModeTypeAllNamespaces,
		v1alpha1.InstallModeTypeSingleNamespace,
	)
	data, err := generateSchema(b, modes, false)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	wnSchema := props["watchNamespace"].(map[string]interface{})

	// Should be anyOf: [null, namespace string schema]
	anyOf, ok := wnSchema["anyOf"].([]interface{})
	require.True(t, ok)
	require.Len(t, anyOf, 2)

	nullSchema := anyOf[0].(map[string]interface{})
	assert.Equal(t, "null", nullSchema["type"])

	nsSchema := anyOf[1].(map[string]interface{})
	assert.Equal(t, "string", nsSchema["type"])
	assert.Equal(t, float64(1), nsSchema["minLength"])
	assert.Equal(t, float64(namespaceNameMaxLength), nsSchema["maxLength"])
	assert.Equal(t, namespaceNamePattern, nsSchema["pattern"])

	// watchNamespace should NOT be required
	if required, ok := schema["required"].([]interface{}); ok {
		assert.NotContains(t, required, "watchNamespace")
	}
}

func TestGenerateSchema_WithWebhooks(t *testing.T) {
	b := makeMinimalBundle()
	modes := sets.New[v1alpha1.InstallModeType](v1alpha1.InstallModeTypeAllNamespaces)
	data, err := generateSchema(b, modes, true)
	require.NoError(t, err)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &schema))

	props := schema["properties"].(map[string]interface{})
	certSchema := props["certProvider"].(map[string]interface{})
	assert.Equal(t, "string", certSchema["type"])

	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "certProvider")
}
