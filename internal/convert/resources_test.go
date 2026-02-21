package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResourceCreatorOptions_ApplyTo(t *testing.T) {
	t.Run("nil object returns nil", func(t *testing.T) {
		opts := ResourceCreatorOptions{func(obj client.Object) {
			obj.SetName("should-not-be-called")
		}}
		result := opts.ApplyTo(nil)
		assert.Nil(t, result)
	})

	t.Run("nil option is skipped", func(t *testing.T) {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "original"},
		}
		opts := ResourceCreatorOptions{nil}
		result := opts.ApplyTo(sa)
		require.NotNil(t, result)
		assert.Equal(t, "original", result.GetName())
	})

	t.Run("options applied in order", func(t *testing.T) {
		sa := &corev1.ServiceAccount{}
		opts := ResourceCreatorOptions{
			func(obj client.Object) { obj.SetName("first") },
			func(obj client.Object) { obj.SetName("second") },
		}
		result := opts.ApplyTo(sa)
		require.NotNil(t, result)
		assert.Equal(t, "second", result.GetName())
	})
}

func TestCreateServiceAccountResource(t *testing.T) {
	sa := CreateServiceAccountResource("test-sa", "test-ns")
	require.NotNil(t, sa)
	assert.Equal(t, "ServiceAccount", sa.Kind)
	assert.Equal(t, "v1", sa.APIVersion)
	assert.Equal(t, "test-sa", sa.Name)
	assert.Equal(t, "test-ns", sa.Namespace)
}

func TestCreateRoleResource(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
	}
	role := CreateRoleResource("test-role", "test-ns", WithRules(rules...))
	require.NotNil(t, role)
	assert.Equal(t, "Role", role.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", role.APIVersion)
	assert.Equal(t, "test-role", role.Name)
	assert.Equal(t, "test-ns", role.Namespace)
	assert.Equal(t, rules, role.Rules)
}

func TestCreateClusterRoleResource(t *testing.T) {
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get"}},
	}
	cr := CreateClusterRoleResource("test-cr", WithRules(rules...))
	require.NotNil(t, cr)
	assert.Equal(t, "ClusterRole", cr.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", cr.APIVersion)
	assert.Equal(t, "test-cr", cr.Name)
	assert.Empty(t, cr.Namespace, "ClusterRole should not have a namespace")
	assert.Equal(t, rules, cr.Rules)
}

func TestCreateClusterRoleBindingResource(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "test-sa", Namespace: "test-ns"},
	}
	roleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "test-cr",
	}
	crb := CreateClusterRoleBindingResource("test-crb",
		WithSubjects(subjects...),
		WithRoleRef(roleRef),
	)
	require.NotNil(t, crb)
	assert.Equal(t, "ClusterRoleBinding", crb.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", crb.APIVersion)
	assert.Equal(t, "test-crb", crb.Name)
	assert.Empty(t, crb.Namespace)
	assert.Equal(t, subjects, crb.Subjects)
	assert.Equal(t, roleRef, crb.RoleRef)
}

func TestCreateRoleBindingResource(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "test-sa", Namespace: "test-ns"},
	}
	roleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "test-role",
	}
	rb := CreateRoleBindingResource("test-rb", "test-ns",
		WithSubjects(subjects...),
		WithRoleRef(roleRef),
	)
	require.NotNil(t, rb)
	assert.Equal(t, "RoleBinding", rb.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", rb.APIVersion)
	assert.Equal(t, "test-rb", rb.Name)
	assert.Equal(t, "test-ns", rb.Namespace)
	assert.Equal(t, subjects, rb.Subjects)
	assert.Equal(t, roleRef, rb.RoleRef)
}

func TestCreateDeploymentResource(t *testing.T) {
	labels := map[string]string{"app": "test"}
	depSpec := appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec:       corev1.PodSpec{},
		},
	}
	dep := CreateDeploymentResource("test-dep", "test-ns",
		WithDeploymentSpec(depSpec),
		WithLabels(labels),
	)
	require.NotNil(t, dep)
	assert.Equal(t, "Deployment", dep.Kind)
	assert.Equal(t, "apps/v1", dep.APIVersion)
	assert.Equal(t, "test-dep", dep.Name)
	assert.Equal(t, "test-ns", dep.Namespace)
	assert.Equal(t, depSpec, dep.Spec)
	assert.Equal(t, labels, dep.Labels)
}

func TestCreateServiceResource(t *testing.T) {
	svcSpec := corev1.ServiceSpec{
		Ports: []corev1.ServicePort{
			{Port: 443, Name: "https"},
		},
	}
	svc := CreateServiceResource("test-svc", "test-ns", WithServiceSpec(svcSpec))
	require.NotNil(t, svc)
	assert.Equal(t, "Service", svc.Kind)
	assert.Equal(t, "v1", svc.APIVersion)
	assert.Equal(t, "test-svc", svc.Name)
	assert.Equal(t, "test-ns", svc.Namespace)
	assert.Equal(t, svcSpec, svc.Spec)
}

func TestCreateValidatingWebhookConfigurationResource(t *testing.T) {
	sideEffects := admissionregistrationv1.SideEffectClassNone
	webhooks := []admissionregistrationv1.ValidatingWebhook{
		{
			Name:                    "validate.test.io",
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
		},
	}
	vwc := CreateValidatingWebhookConfigurationResource("test-vwc", "test-ns",
		WithValidatingWebhooks(webhooks...),
	)
	require.NotNil(t, vwc)
	assert.Equal(t, "ValidatingWebhookConfiguration", vwc.Kind)
	assert.Equal(t, "admissionregistration.k8s.io/v1", vwc.APIVersion)
	assert.Equal(t, "test-vwc", vwc.Name)
	assert.Equal(t, "test-ns", vwc.Namespace)
	assert.Equal(t, webhooks, vwc.Webhooks)
}

func TestCreateMutatingWebhookConfigurationResource(t *testing.T) {
	sideEffects := admissionregistrationv1.SideEffectClassNone
	webhooks := []admissionregistrationv1.MutatingWebhook{
		{
			Name:                    "mutate.test.io",
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
		},
	}
	mwc := CreateMutatingWebhookConfigurationResource("test-mwc", "test-ns",
		WithMutatingWebhooks(webhooks...),
	)
	require.NotNil(t, mwc)
	assert.Equal(t, "MutatingWebhookConfiguration", mwc.Kind)
	assert.Equal(t, "admissionregistration.k8s.io/v1", mwc.APIVersion)
	assert.Equal(t, "test-mwc", mwc.Name)
	assert.Equal(t, "test-ns", mwc.Namespace)
	assert.Equal(t, webhooks, mwc.Webhooks)
}
