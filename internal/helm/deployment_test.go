package helm

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/joelanford/orb/internal/bundle"
)

func boolPtr(b bool) *bool { return &b }

func makeDeploymentBundle(podSpec corev1.PodSpec, withWebhook bool) *bundle.RegistryV1 {
	if podSpec.Containers == nil {
		podSpec.Containers = []corev1.Container{
			{Name: "manager", Image: "registry.io/test-operator:v1.0.0"},
		}
	}
	return makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = []v1alpha1.StrategyDeploymentSpec{
			{
				Name: "controller-manager",
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: podSpec,
					},
				},
			},
		}
		if withWebhook {
			failPolicy := admissionregistrationv1.Fail
			sideEffects := admissionregistrationv1.SideEffectClassNone
			path := "/validate"
			b.CSV.Spec.WebhookDefinitions = []v1alpha1.WebhookDescription{
				{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate-test",
					DeploymentName:          "controller-manager",
					ContainerPort:           443,
					WebhookPath:             &path,
					FailurePolicy:           &failPolicy,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				},
			}
		}
	})
}

// renderDeployment generates a helm chart from the bundle, renders the
// deployment template with the given values, and parses the result as a
// Deployment.
func renderDeployment(t *testing.T, b *bundle.RegistryV1, valOverrides map[string]any) appsv1.Deployment {
	t.Helper()

	rendered, err := renderChart(t, b, valOverrides)
	require.NoError(t, err)

	var depYAML string
	for name, data := range rendered {
		if strings.HasSuffix(name, "deployment.yaml") {
			depYAML = data
			break
		}
	}
	require.NotEmpty(t, depYAML, "deployment.yaml not found in rendered output")

	var dep appsv1.Deployment
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(depYAML), 4096)
	for {
		var d appsv1.Deployment
		if err := decoder.Decode(&d); err != nil {
			break
		}
		if d.Kind == "Deployment" || d.Name != "" {
			dep = d
			break
		}
	}
	require.NotEmpty(t, dep.Name, "failed to parse Deployment from rendered YAML:\n%s", depYAML)
	return dep
}

// --- VolumeMounts ---

func TestDeploymentVolumeMounts_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "data", MountPath: "/data", ReadOnly: true},
				{Name: "config", MountPath: "/etc/config"},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 2)
	assert.Equal(t, "data", mounts[0].Name)
	assert.Equal(t, "/data", mounts[0].MountPath)
	assert.True(t, mounts[0].ReadOnly)
	assert.Equal(t, "config", mounts[1].Name)
	assert.Equal(t, "/etc/config", mounts[1].MountPath)
}

func TestDeploymentVolumeMounts_CertOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, len(certVolumeConfigList))
	for i, cfg := range certVolumeConfigList {
		assert.Equal(t, cfg.Name, mounts[i].Name)
		assert.Equal(t, cfg.Path, mounts[i].MountPath)
	}
}

func TestDeploymentVolumeMounts_BaseAndCert(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			VolumeMounts: []corev1.VolumeMount{
				{Name: "data", MountPath: "/data", ReadOnly: true},
			},
		}},
	}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 1+len(certVolumeConfigList))
	assert.Equal(t, "data", mounts[0].Name)
	for i, cfg := range certVolumeConfigList {
		assert.Equal(t, cfg.Name, mounts[1+i].Name)
		assert.Equal(t, cfg.Path, mounts[1+i].MountPath)
	}
}

func TestDeploymentVolumeMounts_CertDisabled(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:         "manager",
			Image:        "registry.io/test-operator:v1.0.0",
			VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
		}},
	}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "",
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 1)
	assert.Equal(t, "data", mounts[0].Name)
}

func TestDeploymentVolumeMounts_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"volumeMounts": []any{
				map[string]any{"name": "extra", "mountPath": "/extra"},
			},
		},
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 1)
	assert.Equal(t, "extra", mounts[0].Name)
	assert.Equal(t, "/extra", mounts[0].MountPath)
}

func TestDeploymentVolumeMounts_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:         "manager",
			Image:        "registry.io/test-operator:v1.0.0",
			VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"volumeMounts": []any{
				map[string]any{"name": "extra", "mountPath": "/extra"},
			},
		},
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 2)
	assert.Equal(t, "data", mounts[0].Name)
	assert.Equal(t, "extra", mounts[1].Name)
}

func TestDeploymentVolumeMounts_BaseAndCertAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:         "manager",
			Image:        "registry.io/test-operator:v1.0.0",
			VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
		}},
	}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
		"deploymentConfig": map[string]any{
			"volumeMounts": []any{
				map[string]any{"name": "extra", "mountPath": "/extra"},
			},
		},
	})
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	require.Len(t, mounts, 1+len(certVolumeConfigList)+1)
	assert.Equal(t, "data", mounts[0].Name)
	for i, cfg := range certVolumeConfigList {
		assert.Equal(t, cfg.Name, mounts[1+i].Name)
	}
	assert.Equal(t, "extra", mounts[1+len(certVolumeConfigList)].Name)
}

// --- Volumes ---

func TestDeploymentVolumes_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Volumes: []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "config", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, 2)
	assert.Equal(t, "data", vols[0].Name)
	assert.Equal(t, "config", vols[1].Name)
}

func TestDeploymentVolumes_CertOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, len(certVolumeConfigList))
	for i, cfg := range certVolumeConfigList {
		assert.Equal(t, cfg.Name, vols[i].Name)
		require.NotNil(t, vols[i].Secret)
	}
}

func TestDeploymentVolumes_BaseAndCert(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Volumes: []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "cert-manager",
	})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, 1+len(certVolumeConfigList))
	assert.Equal(t, "data", vols[0].Name)
	for i, cfg := range certVolumeConfigList {
		assert.Equal(t, cfg.Name, vols[1+i].Name)
		require.NotNil(t, vols[1+i].Secret)
	}
}

func TestDeploymentVolumes_CertDisabled(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Volumes: []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}, true)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"certProvider":   "",
	})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, 1)
	assert.Equal(t, "data", vols[0].Name)
}

func TestDeploymentVolumes_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"volumes": []any{
				map[string]any{"name": "extra", "emptyDir": map[string]any{}},
			},
		},
	})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, 1)
	assert.Equal(t, "extra", vols[0].Name)
}

func TestDeploymentVolumes_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Volumes: []corev1.Volume{
			{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"volumes": []any{
				map[string]any{"name": "extra", "emptyDir": map[string]any{}},
			},
		},
	})
	vols := dep.Spec.Template.Spec.Volumes
	require.Len(t, vols, 2)
	assert.Equal(t, "data", vols[0].Name)
	assert.Equal(t, "extra", vols[1].Name)
}

// --- Tolerations ---

func TestDeploymentTolerations_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{Key: "node-role", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	tols := dep.Spec.Template.Spec.Tolerations
	require.Len(t, tols, 1)
	assert.Equal(t, "node-role", tols[0].Key)
}

func TestDeploymentTolerations_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"tolerations": []any{
				map[string]any{"key": "gpu", "operator": "Exists", "effect": "NoSchedule"},
			},
		},
	})
	tols := dep.Spec.Template.Spec.Tolerations
	require.Len(t, tols, 1)
	assert.Equal(t, "gpu", tols[0].Key)
}

func TestDeploymentTolerations_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{Key: "node-role", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"tolerations": []any{
				map[string]any{"key": "gpu", "operator": "Exists", "effect": "NoSchedule"},
			},
		},
	})
	tols := dep.Spec.Template.Spec.Tolerations
	require.Len(t, tols, 2)
	assert.Equal(t, "node-role", tols[0].Key)
	assert.Equal(t, "gpu", tols[1].Key)
}

// --- EnvFrom ---

func TestDeploymentEnvFrom_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "base-cm"}}},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	envFrom := dep.Spec.Template.Spec.Containers[0].EnvFrom
	require.Len(t, envFrom, 1)
	require.NotNil(t, envFrom[0].ConfigMapRef)
	assert.Equal(t, "base-cm", envFrom[0].ConfigMapRef.Name)
}

func TestDeploymentEnvFrom_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"envFrom": []any{
				map[string]any{"configMapRef": map[string]any{"name": "config-cm"}},
			},
		},
	})
	envFrom := dep.Spec.Template.Spec.Containers[0].EnvFrom
	require.Len(t, envFrom, 1)
	require.NotNil(t, envFrom[0].ConfigMapRef)
	assert.Equal(t, "config-cm", envFrom[0].ConfigMapRef.Name)
}

func TestDeploymentEnvFrom_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "base-cm"}}},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"envFrom": []any{
				map[string]any{"configMapRef": map[string]any{"name": "config-cm"}},
			},
		},
	})
	envFrom := dep.Spec.Template.Spec.Containers[0].EnvFrom
	require.Len(t, envFrom, 2)
	require.NotNil(t, envFrom[0].ConfigMapRef)
	assert.Equal(t, "base-cm", envFrom[0].ConfigMapRef.Name)
	require.NotNil(t, envFrom[1].ConfigMapRef)
	assert.Equal(t, "config-cm", envFrom[1].ConfigMapRef.Name)
}

// --- Env ---

func TestDeploymentEnv_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			Env: []corev1.EnvVar{
				{Name: "FOO", Value: "bar"},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	env := dep.Spec.Template.Spec.Containers[0].Env
	require.Len(t, env, 1)
	assert.Equal(t, "FOO", env[0].Name)
	assert.Equal(t, "bar", env[0].Value)
}

func TestDeploymentEnv_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"env": []any{
				map[string]any{"name": "BAZ", "value": "qux"},
			},
		},
	})
	env := dep.Spec.Template.Spec.Containers[0].Env
	require.Len(t, env, 1)
	assert.Equal(t, "BAZ", env[0].Name)
	assert.Equal(t, "qux", env[0].Value)
}

func TestDeploymentEnv_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			Env: []corev1.EnvVar{
				{Name: "BASE_ONLY", Value: "base-val"},
				{Name: "SHARED", Value: "base-val"},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"env": []any{
				map[string]any{"name": "SHARED", "value": "config-val"},
				map[string]any{"name": "CONFIG_ONLY", "value": "config-val"},
			},
		},
	})
	env := dep.Spec.Template.Spec.Containers[0].Env
	require.Len(t, env, 3)
	// base-only preserved
	assert.Equal(t, "BASE_ONLY", env[0].Name)
	assert.Equal(t, "base-val", env[0].Value)
	// shared: config wins
	assert.Equal(t, "SHARED", env[1].Name)
	assert.Equal(t, "config-val", env[1].Value)
	// config-only appended
	assert.Equal(t, "CONFIG_ONLY", env[2].Name)
	assert.Equal(t, "config-val", env[2].Value)
}

// --- Resources ---

func TestDeploymentResources_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	res := dep.Spec.Template.Spec.Containers[0].Resources
	assert.Equal(t, resource.MustParse("100m"), res.Requests[corev1.ResourceCPU])
}

func TestDeploymentResources_ConfigOverridesBase(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "manager",
			Image: "registry.io/test-operator:v1.0.0",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	})
	res := dep.Spec.Template.Spec.Containers[0].Resources
	assert.Equal(t, resource.MustParse("500m"), res.Requests[corev1.ResourceCPU])
}

// --- NodeSelector ---

func TestDeploymentNodeSelector_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		NodeSelector: map[string]string{"disktype": "ssd"},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	assert.Equal(t, map[string]string{"disktype": "ssd"}, dep.Spec.Template.Spec.NodeSelector)
}

func TestDeploymentNodeSelector_ConfigOverridesBase(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		NodeSelector: map[string]string{"disktype": "ssd"},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"nodeSelector": map[string]any{"zone": "us-east-1a"},
		},
	})
	assert.Equal(t, map[string]string{"zone": "us-east-1a"}, dep.Spec.Template.Spec.NodeSelector)
}

// --- Affinity ---

func TestDeploymentAffinity_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "kubernetes.io/os",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						}},
					}},
				},
			},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	require.NotNil(t, dep.Spec.Template.Spec.Affinity)
	require.NotNil(t, dep.Spec.Template.Spec.Affinity.NodeAffinity)
}

func TestDeploymentAffinity_ConfigOverridesBase(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "kubernetes.io/os",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						}},
					}},
				},
			},
		},
	}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"affinity": map[string]any{
				"nodeAffinity": map[string]any{
					"requiredDuringSchedulingIgnoredDuringExecution": map[string]any{
						"nodeSelectorTerms": []any{map[string]any{
							"matchExpressions": []any{map[string]any{
								"key":      "kubernetes.io/arch",
								"operator": "In",
								"values":   []any{"amd64"},
							}},
						}},
					},
				},
			},
		},
	})
	require.NotNil(t, dep.Spec.Template.Spec.Affinity)
	require.NotNil(t, dep.Spec.Template.Spec.Affinity.NodeAffinity)
	terms := dep.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	require.Len(t, terms, 1)
	assert.Equal(t, "kubernetes.io/arch", terms[0].MatchExpressions[0].Key)
}

// --- Annotations ---

func TestDeploymentAnnotations_BaseOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)
	b.CSV.Annotations = map[string]string{"base-key": "base-value"}

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	annos := dep.Spec.Template.Annotations
	assert.Equal(t, "base-value", annos["base-key"])
	assert.Contains(t, annos, "olm.targetNamespaces")
}

func TestDeploymentAnnotations_ConfigOnly(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"annotations": map[string]any{"extra-key": "extra-value"},
		},
	})
	annos := dep.Spec.Template.Annotations
	assert.Equal(t, "extra-value", annos["extra-key"])
	assert.Contains(t, annos, "olm.targetNamespaces")
}

func TestDeploymentAnnotations_BaseAndConfig(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)
	b.CSV.Annotations = map[string]string{"base-key": "base-value"}

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"annotations": map[string]any{"extra-key": "extra-value"},
		},
	})
	annos := dep.Spec.Template.Annotations
	assert.Equal(t, "base-value", annos["base-key"])
	assert.Equal(t, "extra-value", annos["extra-key"])
	assert.Contains(t, annos, "olm.targetNamespaces")
}

func TestDeploymentAnnotations_ConfigCannotOverrideBase(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)
	b.CSV.Annotations = map[string]string{"base-key": "base-value"}

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "",
		"deploymentConfig": map[string]any{
			"annotations": map[string]any{"base-key": "overridden"},
		},
	})
	annos := dep.Spec.Template.Annotations
	assert.Equal(t, "base-value", annos["base-key"], "config should not override base annotations")
}

// --- Container optional fields ---

func TestDeploymentContainerBase_AllFields(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:            "manager",
			Image:           "registry.io/test-operator:v1.0.0",
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"/manager"},
			Args:            []string{"--leader-elect", "--health-probe-bind-address=:8081"},
			Ports: []corev1.ContainerPort{
				{Name: "metrics", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt32(8081)},
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{Path: "/readyz", Port: intstr.FromInt32(8081)},
				},
			},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:             boolPtr(true),
				AllowPrivilegeEscalation: boolPtr(false),
			},
		}},
	}, false)

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, corev1.PullAlways, c.ImagePullPolicy)
	assert.Equal(t, []string{"/manager"}, c.Command)
	assert.Equal(t, []string{"--leader-elect", "--health-probe-bind-address=:8081"}, c.Args)
	require.Len(t, c.Ports, 1)
	assert.Equal(t, int32(8080), c.Ports[0].ContainerPort)
	assert.Equal(t, "metrics", c.Ports[0].Name)
	require.NotNil(t, c.LivenessProbe)
	assert.Equal(t, "/healthz", c.LivenessProbe.HTTPGet.Path)
	require.NotNil(t, c.ReadinessProbe)
	assert.Equal(t, "/readyz", c.ReadinessProbe.HTTPGet.Path)
	require.NotNil(t, c.SecurityContext)
	assert.True(t, *c.SecurityContext.RunAsNonRoot)
	assert.False(t, *c.SecurityContext.AllowPrivilegeEscalation)
}

// --- Deployment optional fields ---

func TestDeploymentOptionalFields(t *testing.T) {
	replicas := int32(3)
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs = []v1alpha1.StrategyDeploymentSpec{
			{
				Name:  "controller-manager",
				Label: map[string]string{"component": "controller"},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RollingUpdateDeploymentStrategyType,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: "my-sa",
							Containers: []corev1.Container{
								{Name: "manager", Image: "registry.io/test-operator:v1.0.0"},
							},
							InitContainers: []corev1.Container{
								{Name: "init", Image: "registry.io/init:latest"},
							},
						},
					},
				},
			},
		}
	})

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})

	// Labels
	assert.Equal(t, "controller", dep.Labels["component"])

	// Replicas
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(3), *dep.Spec.Replicas)

	// Strategy
	assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, dep.Spec.Strategy.Type)

	// ServiceAccountName
	assert.Equal(t, "my-sa", dep.Spec.Template.Spec.ServiceAccountName)

	// InitContainers
	require.Len(t, dep.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "init", dep.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "registry.io/init:latest", dep.Spec.Template.Spec.InitContainers[0].Image)
}

// --- Annotations ---

func TestDeploymentAnnotations_ConfigCannotOverrideOlmTargetNamespaces(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)

	dep := renderDeployment(t, b, map[string]any{
		"watchNamespace": "real-ns",
		"deploymentConfig": map[string]any{
			"annotations": map[string]any{"olm.targetNamespaces": "should-be-ignored"},
		},
	})
	annos := dep.Spec.Template.Annotations
	assert.Equal(t, "real-ns", annos["olm.targetNamespaces"], "config should not override olm.targetNamespaces")
}

func TestDeploymentAnnotations_YAMLSpecialValuesSurviveRoundTrip(t *testing.T) {
	b := makeDeploymentBundle(corev1.PodSpec{}, false)
	b.CSV.Annotations = map[string]string{
		"certified": "false",
		"count":     "3",
		"nullable":  "null",
		"plain":     "hello",
	}

	dep := renderDeployment(t, b, map[string]any{"watchNamespace": ""})
	annos := dep.Spec.Template.Annotations

	assert.Equal(t, "false", annos["certified"])
	assert.Equal(t, "3", annos["count"])
	assert.Equal(t, "null", annos["nullable"])
	assert.Equal(t, "hello", annos["plain"])
}

func TestValidateWatchNamespace_OwnNamespaceOnly_MatchesRelease(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
		}
	})
	// watchNamespace == release namespace ("test-ns") should pass
	_, err := renderChart(t, b, map[string]any{"watchNamespace": "test-ns"})
	assert.NoError(t, err)
}

func TestValidateWatchNamespace_OwnNamespaceOnly_DiffersFromRelease(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
		}
	})
	// watchNamespace != release namespace should fail
	_, err := renderChart(t, b, map[string]any{"watchNamespace": "other-ns"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must equal the release namespace")
}

func TestValidateWatchNamespace_OwnNamespaceOnly_Empty(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeOwnNamespace, Supported: true},
		}
	})
	// Empty watchNamespace should pass (the condition checks .Values.watchNamespace is truthy)
	_, err := renderChart(t, b, map[string]any{"watchNamespace": ""})
	assert.NoError(t, err)
}

func TestValidateWatchNamespace_SingleNamespaceOnly_DiffersFromRelease(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeSingleNamespace, Supported: true},
		}
	})
	// watchNamespace != release namespace should pass
	_, err := renderChart(t, b, map[string]any{"watchNamespace": "other-ns"})
	assert.NoError(t, err)
}

func TestValidateWatchNamespace_SingleNamespaceOnly_MatchesRelease(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeSingleNamespace, Supported: true},
		}
	})
	// watchNamespace == release namespace should fail
	_, err := renderChart(t, b, map[string]any{"watchNamespace": "test-ns"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must differ from the release namespace")
}

func TestValidateWatchNamespace_AllNamespaces_NoValidation(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
		}
	})
	// Any value should pass — no validation
	_, err := renderChart(t, b, map[string]any{"watchNamespace": ""})
	assert.NoError(t, err)
}

func TestValidateWatchNamespace_AllAndSingle_NoValidation(t *testing.T) {
	b := makeMinimalBundle(func(b *bundle.RegistryV1) {
		b.CSV.Spec.InstallModes = []v1alpha1.InstallMode{
			{Type: v1alpha1.InstallModeTypeAllNamespaces, Supported: true},
			{Type: v1alpha1.InstallModeTypeSingleNamespace, Supported: true},
		}
	})
	// watchNamespace == release namespace should still pass (no validation when both modes)
	_, err := renderChart(t, b, map[string]any{"watchNamespace": "test-ns"})
	assert.NoError(t, err)
}
