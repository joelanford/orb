package helm

import (
	"encoding/json"
	"fmt"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/joelanford/orb/internal/bundle"
)

func generateSchema(b *bundle.RegistryV1, supportedModes sets.Set[v1alpha1.InstallModeType], hasWebhooks bool) ([]byte, error) {
	_ = b // b is available for future use; modes are pre-computed

	watchNSSchema := buildWatchNamespaceSchema(supportedModes)

	properties := map[string]interface{}{
		"watchNamespace":   watchNSSchema,
		"deploymentConfig": buildDeploymentConfigSchema(),
	}

	required := []string{}

	if hasWebhooks {
		properties["certProvider"] = map[string]interface{}{
			"type": "string",
			"enum": []string{"cert-manager", "service-ca"},
		}
		required = append(required, "certProvider")
	}

	schema := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}

	supportsAllNS := supportedModes.Has(v1alpha1.InstallModeTypeAllNamespaces)
	supportsNonAllNS := supportedModes.Has(v1alpha1.InstallModeTypeSingleNamespace) ||
		supportedModes.Has(v1alpha1.InstallModeTypeOwnNamespace)

	if !supportsAllNS && supportsNonAllNS {
		required = append(required, "watchNamespace")
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling schema: %w", err)
	}
	return append(data, '\n'), nil
}

const (
	namespaceNamePattern   = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	namespaceNameMaxLength = 63
)

func namespaceStringSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":      "string",
		"minLength": 1,
		"maxLength": namespaceNameMaxLength,
		"pattern":   namespaceNamePattern,
	}
}

func buildWatchNamespaceSchema(supportedModes sets.Set[v1alpha1.InstallModeType]) map[string]interface{} {
	supportsAllNS := supportedModes.Has(v1alpha1.InstallModeTypeAllNamespaces)
	supportsNonAllNS := supportedModes.Has(v1alpha1.InstallModeTypeSingleNamespace) ||
		supportedModes.Has(v1alpha1.InstallModeTypeOwnNamespace)

	switch {
	case supportsAllNS && !supportsNonAllNS:
		// Only AllNamespaces — value must be empty string
		return map[string]interface{}{
			"type":  "string",
			"const": "",
		}
	case !supportsAllNS && supportsNonAllNS:
		// Only non-AllNamespaces — required, must be a valid namespace name
		return namespaceStringSchema()
	default:
		// Both supported — optional, null or valid namespace name
		return map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{"type": "null"},
				namespaceStringSchema(),
			},
		}
	}
}

func buildDeploymentConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"env": map[string]interface{}{
				"type": "array",
			},
			"envFrom": map[string]interface{}{
				"type": "array",
			},
			"volumes": map[string]interface{}{
				"type": "array",
			},
			"volumeMounts": map[string]interface{}{
				"type": "array",
			},
			"tolerations": map[string]interface{}{
				"type": "array",
			},
			"resources": map[string]interface{}{
				"type": "object",
			},
			"nodeSelector": map[string]interface{}{
				"type": "object",
			},
			"affinity": map[string]interface{}{
				"type": "object",
			},
			"annotations": map[string]interface{}{
				"type": "object",
			},
		},
		"additionalProperties": false,
	}
}
