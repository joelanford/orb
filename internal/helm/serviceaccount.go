package helm

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/joelanford/orb/internal/bundle"
)

func generateServiceAccounts(b *bundle.RegistryV1) ([]byte, error) {
	allPermissions := slices.Concat(
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions,
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions,
	)

	serviceAccountNames := sets.New[string]()
	for _, permission := range allPermissions {
		serviceAccountNames.Insert(saNameOrDefault(permission.ServiceAccountName))
	}

	var sb strings.Builder
	first := true
	for _, saName := range slices.Sorted(slices.Values(serviceAccountNames.UnsortedList())) {
		if saName == "default" {
			continue
		}
		if !first {
			sb.WriteString("---\n")
		}
		first = false
		fmt.Fprintf(&sb, `apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: {{ .Release.Namespace }}
`, saName)
	}

	return []byte(sb.String()), nil
}
