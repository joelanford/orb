package helm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

// rbacObjects holds the parsed RBAC resources from a rendered clusterrole.yaml.
type rbacObjects struct {
	ClusterRoles        []rbacv1.ClusterRole
	ClusterRoleBindings []rbacv1.ClusterRoleBinding
	Roles               []rbacv1.Role
	RoleBindings        []rbacv1.RoleBinding
}

// renderRBACTemplate generates a helm chart from the bundle, renders the
// clusterrole.yaml template with the given values, and parses the result into
// typed RBAC objects.
func renderRBACTemplate(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) rbacObjects {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var rbacYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "clusterrole.yaml") {
			rbacYAML = data
			break
		}
	}
	require.NotEmpty(t, rbacYAML, "clusterrole.yaml not found in rendered output")

	var objs rbacObjects
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(rbacYAML), 4096)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		if len(raw) == 0 {
			continue
		}

		var meta struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}

		switch meta.Kind {
		case "ClusterRole":
			var cr rbacv1.ClusterRole
			require.NoError(t, json.Unmarshal(raw, &cr))
			objs.ClusterRoles = append(objs.ClusterRoles, cr)
		case "ClusterRoleBinding":
			var crb rbacv1.ClusterRoleBinding
			require.NoError(t, json.Unmarshal(raw, &crb))
			objs.ClusterRoleBindings = append(objs.ClusterRoleBindings, crb)
		case "Role":
			var r rbacv1.Role
			require.NoError(t, json.Unmarshal(raw, &r))
			objs.Roles = append(objs.Roles, r)
		case "RoleBinding":
			var rb rbacv1.RoleBinding
			require.NoError(t, json.Unmarshal(raw, &rb))
			objs.RoleBindings = append(objs.RoleBindings, rb)
		}
	}
	return objs
}

// --- ClusterPermissions ---

func TestRBAC_ClusterPermissions(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"secrets"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": ""})

	require.Len(t, objs.ClusterRoles, 1)
	require.Len(t, objs.ClusterRoleBindings, 1)

	cr := objs.ClusterRoles[0]
	require.Len(t, cr.Rules, 1)
	assert.Equal(t, []string{"get", "list"}, cr.Rules[0].Verbs)
	assert.Equal(t, []string{""}, cr.Rules[0].APIGroups)
	assert.Equal(t, []string{"secrets"}, cr.Rules[0].Resources)

	crb := objs.ClusterRoleBindings[0]
	assert.Equal(t, cr.Name, crb.RoleRef.Name)
	assert.Equal(t, "ClusterRole", crb.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", crb.RoleRef.APIGroup)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", crb.Subjects[0].Kind)
	assert.Equal(t, "controller-manager", crb.Subjects[0].Name)
	assert.Equal(t, "test-ns", crb.Subjects[0].Namespace)
}

func TestRBAC_ClusterPermissions_MultipleRules(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"secrets"}},
					{Verbs: []string{"create", "delete"}, APIGroups: []string{"apps"}, Resources: []string{"deployments"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": ""})

	require.Len(t, objs.ClusterRoles, 1)
	cr := objs.ClusterRoles[0]
	require.Len(t, cr.Rules, 2)
	assert.Equal(t, []string{"get"}, cr.Rules[0].Verbs)
	assert.Equal(t, []string{"secrets"}, cr.Rules[0].Resources)
	assert.Equal(t, []string{"create", "delete"}, cr.Rules[1].Verbs)
	assert.Equal(t, []string{"deployments"}, cr.Rules[1].Resources)
}

// --- Permissions (AllNamespaces mode: watchNamespace="") ---

func TestRBAC_Permissions_AllNamespaces(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "my-sa",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": ""})

	// AllNamespaces promotes to ClusterRole/ClusterRoleBinding
	require.Len(t, objs.ClusterRoles, 1)
	require.Len(t, objs.ClusterRoleBindings, 1)
	assert.Empty(t, objs.Roles)
	assert.Empty(t, objs.RoleBindings)

	cr := objs.ClusterRoles[0]
	// Should have original rule + namespaces get/list/watch
	require.Len(t, cr.Rules, 2)
	assert.Equal(t, []string{"get"}, cr.Rules[0].Verbs)
	assert.Equal(t, []string{"configmaps"}, cr.Rules[0].Resources)
	assert.Equal(t, []string{"get", "list", "watch"}, cr.Rules[1].Verbs)
	assert.Equal(t, []string{"namespaces"}, cr.Rules[1].Resources)

	crb := objs.ClusterRoleBindings[0]
	assert.Equal(t, cr.Name, crb.RoleRef.Name)
	assert.Equal(t, "ClusterRole", crb.RoleRef.Kind)
	require.Len(t, crb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", crb.Subjects[0].Kind)
	assert.Equal(t, "my-sa", crb.Subjects[0].Name)
	assert.Equal(t, "test-ns", crb.Subjects[0].Namespace)
}

// --- Permissions (SingleNamespace mode: watchNamespace="some-ns") ---

func TestRBAC_Permissions_SingleNamespace(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "my-sa",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": "some-ns"})

	// SingleNamespace uses Role/RoleBinding
	assert.Empty(t, objs.ClusterRoles)
	assert.Empty(t, objs.ClusterRoleBindings)
	require.Len(t, objs.Roles, 1)
	require.Len(t, objs.RoleBindings, 1)

	role := objs.Roles[0]
	assert.Equal(t, "some-ns", role.Namespace)
	require.Len(t, role.Rules, 1)
	assert.Equal(t, []string{"get"}, role.Rules[0].Verbs)
	assert.Equal(t, []string{"configmaps"}, role.Rules[0].Resources)

	rb := objs.RoleBindings[0]
	assert.Equal(t, "some-ns", rb.Namespace)
	assert.Equal(t, role.Name, rb.RoleRef.Name)
	assert.Equal(t, "Role", rb.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", rb.RoleRef.APIGroup)
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "ServiceAccount", rb.Subjects[0].Kind)
	assert.Equal(t, "my-sa", rb.Subjects[0].Name)
	assert.Equal(t, "test-ns", rb.Subjects[0].Namespace)
}

// --- Both ClusterPermissions and Permissions ---

func TestRBAC_ClusterPermissionsAndPermissions_AllNamespaces(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"nodes"}},
				},
			},
		}
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": ""})

	// ClusterPermissions => 1 ClusterRole + 1 ClusterRoleBinding
	// Permissions (AllNamespaces) => 1 more ClusterRole + 1 more ClusterRoleBinding
	require.Len(t, objs.ClusterRoles, 2)
	require.Len(t, objs.ClusterRoleBindings, 2)
	assert.Empty(t, objs.Roles)
	assert.Empty(t, objs.RoleBindings)

	// First ClusterRole is from clusterPermissions (nodes)
	assert.Equal(t, []string{"get"}, objs.ClusterRoles[0].Rules[0].Verbs)
	assert.Equal(t, []string{"nodes"}, objs.ClusterRoles[0].Rules[0].Resources)

	// Second ClusterRole is from permissions promoted (configmaps + namespaces)
	assert.Equal(t, []string{"configmaps"}, objs.ClusterRoles[1].Rules[0].Resources)
	assert.Equal(t, []string{"namespaces"}, objs.ClusterRoles[1].Rules[1].Resources)
}

func TestRBAC_ClusterPermissionsAndPermissions_SingleNamespace(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"nodes"}},
				},
			},
		}
		b.CSV.Spec.InstallStrategy.StrategySpec.Permissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "controller-manager",
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"configmaps"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": "target-ns"})

	// ClusterPermissions => 1 ClusterRole + 1 ClusterRoleBinding (always cluster-scoped)
	require.Len(t, objs.ClusterRoles, 1)
	require.Len(t, objs.ClusterRoleBindings, 1)
	assert.Equal(t, []string{"nodes"}, objs.ClusterRoles[0].Rules[0].Resources)

	// Permissions (SingleNamespace) => Role + RoleBinding
	require.Len(t, objs.Roles, 1)
	require.Len(t, objs.RoleBindings, 1)
	assert.Equal(t, "target-ns", objs.Roles[0].Namespace)
	assert.Equal(t, []string{"configmaps"}, objs.Roles[0].Rules[0].Resources)
	assert.Equal(t, "target-ns", objs.RoleBindings[0].Namespace)
}

// --- Default service account name ---

func TestRBAC_DefaultServiceAccountName(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions = []v1alpha1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "", // empty => "default"
				Rules: []rbacv1.PolicyRule{
					{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"secrets"}},
				},
			},
		}
	})

	objs := renderRBACTemplate(t, b, map[string]any{"watchNamespace": ""})

	require.Len(t, objs.ClusterRoleBindings, 1)
	require.Len(t, objs.ClusterRoleBindings[0].Subjects, 1)
	assert.Equal(t, "default", objs.ClusterRoleBindings[0].Subjects[0].Name)
}
