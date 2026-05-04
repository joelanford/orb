package helm

import (
	"fmt"
	"strings"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/convert"
)

func generateRBAC(b *registryv1.Bundle) ([]byte, error) {
	var sb strings.Builder

	// 1. ClusterPermissions — always ClusterRole/ClusterRoleBinding (unconditional)
	for _, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions {
		saName := saNameOrDefault(perm.ServiceAccountName)
		name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), perm)

		sb.WriteString("---\n")

		rulesYAML := renderRules(perm.Rules)

		fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
%s
`, name, rulesYAML)

		sb.WriteString("---\n")
		fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
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
`, name, name, saName)
	}

	// 2. Permissions — conditional on watchNamespace
	if len(b.CSV.Spec.InstallStrategy.StrategySpec.Permissions) > 0 {
		sb.WriteString("---\n")

		// AllNamespaces mode: promote to ClusterRole + add namespace get/list/watch
		sb.WriteString("{{- if eq .Values.watchNamespace \"\" }}\n")
		for i, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			saName := saNameOrDefault(perm.ServiceAccountName)

			// Add namespaces get/list/watch rule for AllNamespaces
			allNSPerm := perm.DeepCopy()
			allNSPerm.Rules = append(allNSPerm.Rules, rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
			})

			name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), *allNSPerm)

			if i > 0 {
				sb.WriteString("---\n")
			}

			rulesYAML := renderRules(allNSPerm.Rules)

			fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
%s
`, name, rulesYAML)

			sb.WriteString("---\n")
			fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
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
`, name, name, saName)
		}

		// Non-AllNamespaces mode: Role + RoleBinding in watchNamespace
		sb.WriteString("{{- else }}\n")
		for i, perm := range b.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			saName := saNameOrDefault(perm.ServiceAccountName)
			name := convert.DefaultUniqueNameGenerator(fmt.Sprintf("%s-%s", b.CSV.Name, saName), perm)

			if i > 0 {
				sb.WriteString("---\n")
			}

			rulesYAML := renderRules(perm.Rules)

			fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s
  namespace: {{ .Values.watchNamespace }}
rules:
%s
`, name, rulesYAML)

			sb.WriteString("---\n")
			fmt.Fprintf(&sb, `apiVersion: rbac.authorization.k8s.io/v1
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
`, name, name, saName)
		}

		sb.WriteString("{{- end }}\n")
	}

	return []byte(sb.String()), nil
}

// renderRules marshals RBAC rules to indented YAML lines.
// renderRules marshals RBAC PolicyRules to YAML. Marshal will not error
// on []rbacv1.PolicyRule since it contains only primitive fields.
func renderRules(rules []rbacv1.PolicyRule) string {
	data, _ := yaml.Marshal(rules)
	s := strings.TrimRight(string(data), "\n")
	return escapeHelm(s)
}
