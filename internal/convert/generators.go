package convert

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	registrybundle "github.com/operator-framework/operator-registry/pkg/lib/bundle"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	labelKubernetesNamespaceMetadataName = "kubernetes.io/metadata.name"
)

type certVolumeConfig struct {
	Name        string
	Path        string
	TLSCertPath string
	TLSKeyPath  string
}

var certVolumeConfigs = []certVolumeConfig{
	{
		Name:        "webhook-cert",
		Path:        "/tmp/k8s-webhook-server/serving-certs",
		TLSCertPath: "tls.crt",
		TLSKeyPath:  "tls.key",
	}, {
		Name:        "apiservice-cert",
		Path:        "/apiserver.local.config/certificates",
		TLSCertPath: "apiserver.crt",
		TLSKeyPath:  "apiserver.key",
	},
}

// BundleCSVDeploymentGenerator generates all deployments defined in rv1's cluster service version (CSV).
func BundleCSVDeploymentGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	webhookDeployments := sets.Set[string]{}
	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		webhookDeployments.Insert(wh.DeploymentName)
	}

	objs := make([]client.Object, 0, len(rv1.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs))
	for _, depSpec := range rv1.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		annotations := MergeMaps(rv1.CSV.Annotations, depSpec.Spec.Template.Annotations)
		annotations["olm.targetNamespaces"] = strings.Join(opts.TargetNamespaces, ",")
		depSpec.Spec.Template.Annotations = annotations

		depSpec.Spec.RevisionHistoryLimit = ptr.To(int32(1))

		deploymentResource := CreateDeploymentResource(
			depSpec.Name,
			opts.InstallNamespace,
			WithDeploymentSpec(depSpec.Spec),
			WithLabels(depSpec.Label),
		)

		secretInfo := CertProvisionerFor(depSpec.Name, opts).GetCertSecretInfo()
		if webhookDeployments.Has(depSpec.Name) && secretInfo != nil {
			ensureCorrectDeploymentCertVolumes(deploymentResource, *secretInfo)
		}

		objs = append(objs, deploymentResource)
	}
	return objs, nil
}

// BundleCSVPermissionsGenerator generates the Roles and RoleBindings based on bundle's cluster service version
// permission spec.
func BundleCSVPermissionsGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	if len(opts.TargetNamespaces) == 1 && opts.TargetNamespaces[0] == "" {
		return nil, nil
	}

	permissions := rv1.CSV.Spec.InstallStrategy.StrategySpec.Permissions

	objs := make([]client.Object, 0, 2*len(opts.TargetNamespaces)*len(permissions))
	for _, ns := range opts.TargetNamespaces {
		for _, permission := range permissions {
			saName := saNameOrDefault(permission.ServiceAccountName)
			name := opts.UniqueNameGenerator(fmt.Sprintf("%s-%s", rv1.CSV.Name, saName), permission)

			objs = append(objs,
				CreateRoleResource(name, ns, WithRules(permission.Rules...)),
				CreateRoleBindingResource(
					name,
					ns,
					WithSubjects(rbacv1.Subject{Kind: "ServiceAccount", Namespace: opts.InstallNamespace, Name: saName}),
					WithRoleRef(rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}),
				),
			)
		}
	}
	return objs, nil
}

// BundleCSVClusterPermissionsGenerator generates ClusterRoles and ClusterRoleBindings based on the bundle's
// cluster service version clusterPermission spec.
func BundleCSVClusterPermissionsGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}
	clusterPermissions := rv1.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions

	if len(opts.TargetNamespaces) == 1 && opts.TargetNamespaces[0] == "" {
		for _, p := range rv1.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			p.Rules = append(p.Rules, rbacv1.PolicyRule{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"namespaces"},
			})
			clusterPermissions = append(clusterPermissions, p)
		}
	}

	objs := make([]client.Object, 0, 2*len(clusterPermissions))
	for _, permission := range clusterPermissions {
		saName := saNameOrDefault(permission.ServiceAccountName)
		name := opts.UniqueNameGenerator(fmt.Sprintf("%s-%s", rv1.CSV.Name, saName), permission)
		objs = append(objs,
			CreateClusterRoleResource(name, WithRules(permission.Rules...)),
			CreateClusterRoleBindingResource(
				name,
				WithSubjects(rbacv1.Subject{Kind: "ServiceAccount", Namespace: opts.InstallNamespace, Name: saName}),
				WithRoleRef(rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name}),
			),
		)
	}
	return objs, nil
}

// BundleCSVServiceAccountGenerator generates ServiceAccount resources based on the bundle's cluster service version
// permission and clusterPermission spec.
func BundleCSVServiceAccountGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}
	allPermissions := slices.Concat(
		rv1.CSV.Spec.InstallStrategy.StrategySpec.Permissions,
		rv1.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions,
	)

	serviceAccountNames := sets.Set[string]{}
	for _, permission := range allPermissions {
		serviceAccountNames.Insert(saNameOrDefault(permission.ServiceAccountName))
	}

	objs := make([]client.Object, 0, len(serviceAccountNames))
	for _, serviceAccountName := range serviceAccountNames.UnsortedList() {
		if serviceAccountName != "default" {
			objs = append(objs, CreateServiceAccountResource(serviceAccountName, opts.InstallNamespace))
		}
	}
	return objs, nil
}

// BundleCRDGenerator generates CustomResourceDefinition resources from the registry+v1 bundle.
func BundleCRDGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	crdToDeploymentMap := map[string]v1alpha1.WebhookDescription{}
	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		if wh.Type != v1alpha1.ConversionWebhook {
			continue
		}
		for _, crdName := range wh.ConversionCRDs {
			if _, ok := crdToDeploymentMap[crdName]; ok {
				return nil, fmt.Errorf("custom resource definition '%s' is referenced by multiple conversion webhook definitions", crdName)
			}
			crdToDeploymentMap[crdName] = wh
		}
	}

	objs := make([]client.Object, 0, len(rv1.CRDs))
	for _, crd := range rv1.CRDs {
		cp := crd.DeepCopy()
		if cw, ok := crdToDeploymentMap[crd.Name]; ok {
			if crd.Spec.PreserveUnknownFields {
				return nil, fmt.Errorf("custom resource definition '%s' must have .spec.preserveUnknownFields set to false to let API Server call webhook to do the conversion", crd.Name)
			}

			conversionWebhookPath := "/"
			if cw.WebhookPath != nil {
				conversionWebhookPath = *cw.WebhookPath
			}

			certProvisioner := CertProvisionerFor(cw.DeploymentName, opts)
			cp.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ClientConfig: &apiextensionsv1.WebhookClientConfig{
						Service: &apiextensionsv1.ServiceReference{
							Namespace: opts.InstallNamespace,
							Name:      certProvisioner.ServiceName,
							Path:      &conversionWebhookPath,
							Port:      &cw.ContainerPort,
						},
					},
					ConversionReviewVersions: cw.AdmissionReviewVersions,
				},
			}

			if err := certProvisioner.InjectCABundle(cp); err != nil {
				return nil, err
			}
		}
		objs = append(objs, cp)
	}
	return objs, nil
}

// BundleAdditionalResourcesGenerator generates resources for the additional resources included in the bundle.
func BundleAdditionalResourcesGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}
	objs := make([]client.Object, 0, len(rv1.Others))
	for _, res := range rv1.Others {
		supported, namespaced := registrybundle.IsSupported(res.GetKind())
		if !supported {
			return nil, fmt.Errorf("bundle contains unsupported resource: Name: %v, Kind: %v", res.GetName(), res.GetKind())
		}

		obj := res.DeepCopy()
		if namespaced {
			obj.SetNamespace(opts.InstallNamespace)
		}

		objs = append(objs, obj)
	}
	return objs, nil
}

// BundleValidatingWebhookResourceGenerator generates ValidatingAdmissionWebhookConfiguration resources.
func BundleValidatingWebhookResourceGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	//nolint:prealloc
	var objs []client.Object

	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		if wh.Type != v1alpha1.ValidatingAdmissionWebhook {
			continue
		}
		certProvisioner := CertProvisionerFor(wh.DeploymentName, opts)
		webhookName := strings.TrimSuffix(wh.GenerateName, "-")
		webhookResource := CreateValidatingWebhookConfigurationResource(
			webhookName,
			opts.InstallNamespace,
			WithValidatingWebhooks(
				admissionregistrationv1.ValidatingWebhook{
					Name:                    webhookName,
					Rules:                   wh.Rules,
					FailurePolicy:           wh.FailurePolicy,
					MatchPolicy:             wh.MatchPolicy,
					ObjectSelector:          wh.ObjectSelector,
					SideEffects:             wh.SideEffects,
					TimeoutSeconds:          wh.TimeoutSeconds,
					AdmissionReviewVersions: wh.AdmissionReviewVersions,
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Namespace: opts.InstallNamespace,
							Name:      certProvisioner.ServiceName,
							Path:      wh.WebhookPath,
							Port:      &wh.ContainerPort,
						},
					},
					NamespaceSelector: getWebhookNamespaceSelector(opts.TargetNamespaces),
				},
			),
		)
		if err := certProvisioner.InjectCABundle(webhookResource); err != nil {
			return nil, err
		}
		objs = append(objs, webhookResource)
	}
	return objs, nil
}

// BundleMutatingWebhookResourceGenerator generates MutatingAdmissionWebhookConfiguration resources.
func BundleMutatingWebhookResourceGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	//nolint:prealloc
	var objs []client.Object
	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		if wh.Type != v1alpha1.MutatingAdmissionWebhook {
			continue
		}
		certProvisioner := CertProvisionerFor(wh.DeploymentName, opts)
		webhookName := strings.TrimSuffix(wh.GenerateName, "-")
		webhookResource := CreateMutatingWebhookConfigurationResource(
			webhookName,
			opts.InstallNamespace,
			WithMutatingWebhooks(
				admissionregistrationv1.MutatingWebhook{
					Name:                    webhookName,
					Rules:                   wh.Rules,
					FailurePolicy:           wh.FailurePolicy,
					MatchPolicy:             wh.MatchPolicy,
					ObjectSelector:          wh.ObjectSelector,
					SideEffects:             wh.SideEffects,
					TimeoutSeconds:          wh.TimeoutSeconds,
					AdmissionReviewVersions: wh.AdmissionReviewVersions,
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Namespace: opts.InstallNamespace,
							Name:      certProvisioner.ServiceName,
							Path:      wh.WebhookPath,
							Port:      &wh.ContainerPort,
						},
					},
					ReinvocationPolicy: wh.ReinvocationPolicy,
					NamespaceSelector:  getWebhookNamespaceSelector(opts.TargetNamespaces),
				},
			),
		)
		if err := certProvisioner.InjectCABundle(webhookResource); err != nil {
			return nil, err
		}
		objs = append(objs, webhookResource)
	}
	return objs, nil
}

// BundleDeploymentServiceResourceGenerator generates Service resources that support webhooks.
func BundleDeploymentServiceResourceGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	if rv1 == nil {
		return nil, fmt.Errorf("bundle cannot be nil")
	}

	webhookServicePortsByDeployment := map[string]sets.Set[corev1.ServicePort]{}
	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		if _, ok := webhookServicePortsByDeployment[wh.DeploymentName]; !ok {
			webhookServicePortsByDeployment[wh.DeploymentName] = sets.Set[corev1.ServicePort]{}
		}
		webhookServicePortsByDeployment[wh.DeploymentName].Insert(getWebhookServicePort(wh))
	}

	objs := make([]client.Object, 0, len(webhookServicePortsByDeployment))
	for _, deploymentSpec := range rv1.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if _, ok := webhookServicePortsByDeployment[deploymentSpec.Name]; !ok {
			continue
		}

		servicePorts := webhookServicePortsByDeployment[deploymentSpec.Name]
		ports := servicePorts.UnsortedList()
		slices.SortStableFunc(ports, func(a, b corev1.ServicePort) int {
			return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.TargetPort.IntValue(), b.TargetPort.IntValue()))
		})

		var labelSelector map[string]string
		if deploymentSpec.Spec.Selector != nil {
			labelSelector = deploymentSpec.Spec.Selector.MatchLabels
		}

		certProvisioner := CertProvisionerFor(deploymentSpec.Name, opts)
		serviceResource := CreateServiceResource(
			certProvisioner.ServiceName,
			opts.InstallNamespace,
			WithServiceSpec(
				corev1.ServiceSpec{
					Ports:    ports,
					Selector: labelSelector,
				},
			),
		)

		if err := certProvisioner.InjectCABundle(serviceResource); err != nil {
			return nil, err
		}
		objs = append(objs, serviceResource)
	}

	return objs, nil
}

// CertProviderResourceGenerator generates any resources necessary for the CertificateProvider to function correctly.
func CertProviderResourceGenerator(rv1 *registryv1.Bundle, opts Options) ([]client.Object, error) {
	deploymentsWithWebhooks := sets.Set[string]{}

	for _, wh := range rv1.CSV.Spec.WebhookDefinitions {
		deploymentsWithWebhooks.Insert(wh.DeploymentName)
	}

	var objs []client.Object
	for _, depName := range deploymentsWithWebhooks.UnsortedList() {
		certCfg := CertProvisionerFor(depName, opts)
		certObjs, err := certCfg.AdditionalObjects()
		if err != nil {
			return nil, err
		}
		for _, certObj := range certObjs {
			objs = append(objs, &certObj)
		}
	}
	return objs, nil
}

func saNameOrDefault(saName string) string {
	return cmp.Or(saName, "default")
}

func getWebhookServicePort(wh v1alpha1.WebhookDescription) corev1.ServicePort {
	containerPort := int32(443)
	if wh.ContainerPort > 0 {
		containerPort = wh.ContainerPort
	}

	targetPort := intstr.FromInt32(containerPort)
	if wh.TargetPort != nil {
		targetPort = *wh.TargetPort
	}

	return corev1.ServicePort{
		Name:       strconv.Itoa(int(containerPort)),
		Port:       containerPort,
		TargetPort: targetPort,
	}
}

func ensureCorrectDeploymentCertVolumes(dep *appsv1.Deployment, certSecretInfo CertSecretInfo) {
	volumesToRemove := sets.New[string]()
	protectedVolumePaths := sets.New[string]()
	certVolumes := make([]corev1.Volume, 0, len(certVolumeConfigs))
	certVolumeMounts := make([]corev1.VolumeMount, 0, len(certVolumeConfigs))
	for _, cfg := range certVolumeConfigs {
		volumesToRemove.Insert(cfg.Name)
		protectedVolumePaths.Insert(cfg.Path)
		certVolumes = append(certVolumes, corev1.Volume{
			Name: cfg.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: certSecretInfo.SecretName,
					Items: []corev1.KeyToPath{
						{
							Key:  certSecretInfo.CertificateKey,
							Path: cfg.TLSCertPath,
						},
						{
							Key:  certSecretInfo.PrivateKeyKey,
							Path: cfg.TLSKeyPath,
						},
					},
				},
			},
		})
		certVolumeMounts = append(certVolumeMounts, corev1.VolumeMount{
			Name:      cfg.Name,
			MountPath: cfg.Path,
		})
	}

	for _, c := range dep.Spec.Template.Spec.Containers {
		for _, containerVolumeMount := range c.VolumeMounts {
			if protectedVolumePaths.Has(containerVolumeMount.MountPath) {
				volumesToRemove.Insert(containerVolumeMount.Name)
			}
		}
	}

	dep.Spec.Template.Spec.Volumes = slices.Concat(
		slices.DeleteFunc(dep.Spec.Template.Spec.Volumes, func(v corev1.Volume) bool {
			return volumesToRemove.Has(v.Name)
		}),
		certVolumes,
	)

	for i := range dep.Spec.Template.Spec.Containers {
		dep.Spec.Template.Spec.Containers[i].VolumeMounts = slices.Concat(
			slices.DeleteFunc(dep.Spec.Template.Spec.Containers[i].VolumeMounts, func(v corev1.VolumeMount) bool {
				return volumesToRemove.Has(v.Name)
			}),
			certVolumeMounts,
		)
	}
}

func getWebhookNamespaceSelector(targetNamespaces []string) *metav1.LabelSelector {
	if len(targetNamespaces) > 0 && !slices.Contains(targetNamespaces, "") {
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      labelKubernetesNamespaceMetadataName,
					Operator: metav1.LabelSelectorOpIn,
					Values:   targetNamespaces,
				},
			},
		}
	}
	return nil
}
