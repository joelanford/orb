package helm

import (
	"fmt"
	"slices"
	"strings"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func generateServiceAccounts(b *registryv1.Bundle) ([]byte, error) {
	allPermissions := slices.Concat(
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions,
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions,
	)

	serviceAccountNames := sets.New[string]()
	for _, permission := range allPermissions {
		serviceAccountNames.Insert(saNameOrDefault(permission.ServiceAccountName))
	}

	var sb strings.Builder
	for _, saName := range slices.Sorted(slices.Values(serviceAccountNames.UnsortedList())) {
		if saName == "default" {
			continue
		}
		sb.WriteString("---\n")
		fmt.Fprintf(&sb, `apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: {{ .Release.Namespace }}
`, saName)
	}

	return []byte(sb.String()), nil
}
