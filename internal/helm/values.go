package helm

import (
	"bytes"
	"fmt"

	"sigs.k8s.io/yaml"
)

// generateValues returns the raw values.yaml content (with comments) and
// the parsed values map. The raw bytes are used by chartutil.Save/SaveDir
// to write values.yaml (preserving comments), while the parsed map is used
// by Helm's template engine at render time.
func generateValues(hasWebhooks bool) ([]byte, map[string]interface{}, error) {
	var buf bytes.Buffer

	buf.WriteString("# watchNamespace sets the namespace the operator watches for resources.\n")
	buf.WriteString("# An empty string means all namespaces.\n")
	buf.WriteString("watchNamespace: \"\"\n")

	if hasWebhooks {
		buf.WriteString("\n# certProvider specifies the certificate provider for webhook TLS.\n")
		buf.WriteString("# Valid values: \"cert-manager\", \"service-ca\"\n")
		buf.WriteString("certProvider: cert-manager\n")
	}

	buf.WriteString("\n# deploymentConfig allows customizing the operator's Deployment.\n")
	buf.WriteString("#\n")
	buf.WriteString("# Field behaviors:\n")
	buf.WriteString("#   annotations   - merged; base annotations take precedence\n")
	buf.WriteString("#   env           - merged by name; overrides take precedence, new vars appended\n")
	buf.WriteString("#   envFrom       - appended to base list\n")
	buf.WriteString("#   resources     - replaces base resource requirements\n")
	buf.WriteString("#   nodeSelector  - replaces base node selector\n")
	buf.WriteString("#   affinity      - selectively overrides by sub-field\n")
	buf.WriteString("#                   (nodeAffinity, podAffinity, podAntiAffinity)\n")
	buf.WriteString("#   tolerations   - appended to base list\n")
	buf.WriteString("#   volumes       - appended to base list\n")
	buf.WriteString("#   volumeMounts  - appended to base list\n")
	buf.WriteString("deploymentConfig: {}\n")

	raw := buf.Bytes()

	var values map[string]interface{}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return nil, nil, fmt.Errorf("parsing generated values.yaml: %w", err)
	}

	return raw, values, nil
}
