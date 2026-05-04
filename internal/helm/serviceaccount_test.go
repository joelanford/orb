package helm

import (
	"strings"
	"testing"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// renderServiceAccounts generates a helm chart from the bundle, renders the
// serviceaccount template with the given values, and parses the result as a
// slice of ServiceAccount structs.
func renderServiceAccounts(t *testing.T, b *registryv1.Bundle) []corev1.ServiceAccount {
	t.Helper()

	rendered, err := renderChart(t, b, map[string]any{})
	require.NoError(t, err)

	var saYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "serviceaccount.yaml") {
			saYAML = data
			break
		}
	}

	if saYAML == "" {
		return nil
	}

	var result []corev1.ServiceAccount
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(saYAML), 4096)
	for {
		var sa corev1.ServiceAccount
		if err := decoder.Decode(&sa); err != nil {
			break
		}
		if sa.Name != "" {
			result = append(result, sa)
		}
	}
	return result
}

func makeServiceAccountBundle(permissions []v1alpha1.StrategyDeploymentPermissions, clusterPermissions []v1alpha1.StrategyDeploymentPermissions) *registryv1.Bundle {
	return makeMinimalBundle(func(b *registryv1.Bundle) {
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = permissions
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = clusterPermissions
	})
}

func TestServiceAccount_NamedSA(t *testing.T) {
	b := makeServiceAccountBundle(
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "my-sa", Rules: []rbacv1.PolicyRule{}},
		},
		nil,
	)

	sas := renderServiceAccounts(t, b)
	require.Len(t, sas, 1)
	assert.Equal(t, "my-sa", sas[0].Name)
	assert.Equal(t, "test-ns", sas[0].Namespace)
}

func TestServiceAccount_DefaultSkipped(t *testing.T) {
	b := makeServiceAccountBundle(
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "", Rules: []rbacv1.PolicyRule{}}, // empty => "default"
		},
		nil,
	)

	sas := renderServiceAccounts(t, b)
	assert.Empty(t, sas, "the 'default' service account should be skipped")
}

func TestServiceAccount_MultipleSorted(t *testing.T) {
	b := makeServiceAccountBundle(
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "bravo", Rules: []rbacv1.PolicyRule{}},
			{ServiceAccountName: "alpha", Rules: []rbacv1.PolicyRule{}},
		},
		nil,
	)

	sas := renderServiceAccounts(t, b)
	require.Len(t, sas, 2)
	assert.Equal(t, "alpha", sas[0].Name, "service accounts should be sorted alphabetically")
	assert.Equal(t, "bravo", sas[1].Name, "service accounts should be sorted alphabetically")
	assert.Equal(t, "test-ns", sas[0].Namespace)
	assert.Equal(t, "test-ns", sas[1].Namespace)
}

func TestServiceAccount_ClusterPermissions(t *testing.T) {
	b := makeServiceAccountBundle(
		nil,
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "cluster-sa", Rules: []rbacv1.PolicyRule{}},
		},
	)

	sas := renderServiceAccounts(t, b)
	require.Len(t, sas, 1)
	assert.Equal(t, "cluster-sa", sas[0].Name)
	assert.Equal(t, "test-ns", sas[0].Namespace)
}

func TestServiceAccount_DeduplicatedAcrossPermissionTypes(t *testing.T) {
	b := makeServiceAccountBundle(
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "shared-sa", Rules: []rbacv1.PolicyRule{}},
		},
		[]v1alpha1.StrategyDeploymentPermissions{
			{ServiceAccountName: "shared-sa", Rules: []rbacv1.PolicyRule{}},
		},
	)

	sas := renderServiceAccounts(t, b)
	require.Len(t, sas, 1, "duplicate service account names should be deduplicated")
	assert.Equal(t, "shared-sa", sas[0].Name)
}
