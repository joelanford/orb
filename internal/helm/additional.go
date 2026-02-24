package helm

import (
	"fmt"
	"strings"

	registrybundle "github.com/operator-framework/operator-registry/pkg/lib/bundle"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

func generateAdditional(b *bundle.RegistryV1) ([]byte, error) {
	if len(b.Others) == 0 {
		return nil, nil
	}

	var sb strings.Builder

	for _, res := range b.Others {
		supported, namespaced := registrybundle.IsSupported(res.GetKind())
		if !supported {
			return nil, fmt.Errorf("bundle contains unsupported resource: Name: %v, Kind: %v", res.GetName(), res.GetKind())
		}

		sb.WriteString("---\n")

		obj := res.DeepCopy()
		if namespaced {
			obj.SetNamespace("HELM_RELEASE_NS")
		}

		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, fmt.Errorf("marshaling additional resource %s/%s: %w", res.GetKind(), res.GetName(), err)
		}

		resYAML := string(data)
		if namespaced {
			resYAML = strings.ReplaceAll(resYAML, "HELM_RELEASE_NS", "{{ .Release.Namespace }}")
		}
		resYAML = escapeHelmExceptDirectives(resYAML)
		sb.WriteString(resYAML)
	}

	return []byte(sb.String()), nil
}
