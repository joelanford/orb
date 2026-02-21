package helm

import (
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
)

func generateRBAC(b *bundle.RegistryV1) ([]byte, error) {
	var sb strings.Builder
	first := true

	// 1. ClusterPermissions — always ClusterRole/ClusterRoleBinding (unconditional)
	for _, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions {
		saName := saNameOrDefault(perm.ServiceAccountName)
		name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), perm)

		if !first {
			sb.WriteString("---\n")
		}
		first = false

		rulesYAML, err := renderRules(perm.Rules, 0)
		if err != nil {
			return nil, fmt.Errorf("rendering clusterPermission rules: %w", err)
		}

		sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
%s
`, name, rulesYAML))

		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: {{ .Release.Namespace }}
`, name, name, saName))
	}

	// 2. Permissions — conditional on watchNamespace
	if len(b.CSV.Spec.InstallStrategy.StrategySpec.Permissions) > 0 {
		if !first {
			sb.WriteString("---\n")
		}
		first = false

		// AllNamespaces mode: promote to ClusterRole + add namespace get/list/watch
		sb.WriteString("{{- if eq .Values.watchNamespace \"\" }}\n")
		firstInBlock := true
		for _, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			saName := saNameOrDefault(perm.ServiceAccountName)

			// Add namespaces get/list/watch rule for AllNamespaces
			allNSPerm := perm.DeepCopy()
			allNSPerm.Rules = append(allNSPerm.Rules, rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
			})

			name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), *allNSPerm)

			if !firstInBlock {
				sb.WriteString("---\n")
			}
			firstInBlock = false

			rulesYAML, err := renderRules(allNSPerm.Rules, 0)
			if err != nil {
				return nil, fmt.Errorf("rendering permission rules: %w", err)
			}

			sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
%s
`, name, rulesYAML))

			sb.WriteString("---\n")
			sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: {{ .Release.Namespace }}
`, name, name, saName))
		}

		// Non-AllNamespaces mode: Role + RoleBinding in watchNamespace
		sb.WriteString("{{- else }}\n")
		firstInBlock = true
		for _, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			saName := saNameOrDefault(perm.ServiceAccountName)
			name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), perm)

			if !firstInBlock {
				sb.WriteString("---\n")
			}
			firstInBlock = false

			rulesYAML, err := renderRules(perm.Rules, 0)
			if err != nil {
				return nil, fmt.Errorf("rendering permission rules: %w", err)
			}

			sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s
  namespace: {{ .Values.watchNamespace }}
rules:
%s
`, name, rulesYAML))

			sb.WriteString("---\n")
			sb.WriteString(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s
  namespace: {{ .Values.watchNamespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: {{ .Release.Namespace }}
`, name, name, saName))
		}

		sb.WriteString("{{- end }}\n")
	}

	return []byte(sb.String()), nil
}

// renderRules marshals RBAC rules to indented YAML lines.
func renderRules(rules []rbacv1.PolicyRule, indent int) (string, error) {
	data, err := yaml.Marshal(rules)
	if err != nil {
		return "", err
	}
	s := strings.TrimRight(string(data), "\n")
	s = escapeHelm(s)
	if indent > 0 {
		prefix := strings.Repeat(" ", indent)
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			if line != "" {
				lines[i] = prefix + line
			}
		}
		s = strings.Join(lines, "\n")
	}
	return s, nil
}
