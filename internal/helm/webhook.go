package helm

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/joelanford/orb/internal/bundle"
)

func generateWebhooks(b *bundle.RegistryV1) ([]byte, error) {
	var sb strings.Builder

	for _, wh := range b.CSV.Spec.WebhookDefinitions {
		if wh.Type == v1alpha1.ConversionWebhook {
			continue // Conversion webhooks are handled in CRDs
		}

		sb.WriteString("---\n")

		svcName := serviceNameForDeployment(wh.DeploymentName)
		certName := certNameForDeployment(wh.DeploymentName)
		webhookName := strings.TrimSuffix(wh.GenerateName, "-")

		var kind string
		switch wh.Type {
		case v1alpha1.ValidatingAdmissionWebhook:
			kind = "ValidatingWebhookConfiguration"
		case v1alpha1.MutatingAdmissionWebhook:
			kind = "MutatingWebhookConfiguration"
		default:
			continue
		}
		writeWebhookConfig(&sb, kind, webhookName, svcName, certName, wh)
	}

	return []byte(sb.String()), nil
}

func writeWebhookConfig(sb *strings.Builder, kind, webhookName, svcName, certName string, wh v1alpha1.WebhookDescription) {
	sb.WriteString("apiVersion: admissionregistration.k8s.io/v1\n")
	fmt.Fprintf(sb, "kind: %s\n", kind)
	sb.WriteString("metadata:\n")
	fmt.Fprintf(sb, "  name: %s\n", webhookName)
	writeWebhookCertAnnotations(sb, certName)

	sb.WriteString("webhooks:\n")
	fmt.Fprintf(sb, "- name: %s\n", webhookName)

	writeWebhookBody(sb, svcName, wh)

	if wh.ReinvocationPolicy != nil {
		fmt.Fprintf(sb, "  reinvocationPolicy: %s\n", string(*wh.ReinvocationPolicy))
	}
}

func writeWebhookBody(sb *strings.Builder, svcName string, wh v1alpha1.WebhookDescription) {
	// ClientConfig
	sb.WriteString("  clientConfig:\n")
	sb.WriteString("    service:\n")
	fmt.Fprintf(sb, "      name: %s\n", svcName)
	sb.WriteString("      namespace: {{ .Release.Namespace }}\n")
	if wh.WebhookPath != nil {
		fmt.Fprintf(sb, "      path: %s\n", *wh.WebhookPath)
	}
	if wh.ContainerPort > 0 {
		fmt.Fprintf(sb, "      port: %d\n", wh.ContainerPort)
	}

	// Rules
	if len(wh.Rules) > 0 {
		writeYAMLField(sb, "rules", 2, wh.Rules)
	}

	// FailurePolicy
	if wh.FailurePolicy != nil {
		fmt.Fprintf(sb, "  failurePolicy: %s\n", string(*wh.FailurePolicy))
	}

	// MatchPolicy
	if wh.MatchPolicy != nil {
		fmt.Fprintf(sb, "  matchPolicy: %s\n", string(*wh.MatchPolicy))
	}

	// SideEffects
	if wh.SideEffects != nil {
		fmt.Fprintf(sb, "  sideEffects: %s\n", string(*wh.SideEffects))
	}

	// TimeoutSeconds
	if wh.TimeoutSeconds != nil {
		fmt.Fprintf(sb, "  timeoutSeconds: %d\n", *wh.TimeoutSeconds)
	}

	// AdmissionReviewVersions
	if len(wh.AdmissionReviewVersions) > 0 {
		sb.WriteString("  admissionReviewVersions:\n")
		for _, v := range wh.AdmissionReviewVersions {
			fmt.Fprintf(sb, "  - %s\n", v)
		}
	}

	// ObjectSelector
	if wh.ObjectSelector != nil {
		writeYAMLField(sb, "objectSelector", 2, wh.ObjectSelector)
	}

	// NamespaceSelector — conditional on watchNamespace
	sb.WriteString("  {{- if ne .Values.watchNamespace \"\" }}\n")
	sb.WriteString("  namespaceSelector:\n")
	sb.WriteString("    matchExpressions:\n")
	sb.WriteString("    - key: kubernetes.io/metadata.name\n")
	sb.WriteString("      operator: In\n")
	sb.WriteString("      values:\n")
	sb.WriteString("      - {{ .Values.watchNamespace }}\n")
	sb.WriteString("  {{- end }}\n")
}

func writeWebhookCertAnnotations(sb *strings.Builder, certName string) {
	sb.WriteString("  {{- if eq .Values.certProvider \"cert-manager\" }}\n")
	sb.WriteString("  annotations:\n")
	fmt.Fprintf(sb, "    cert-manager.io/inject-ca-from: {{ .Release.Namespace }}/%s\n", certName)
	sb.WriteString("  {{- else if eq .Values.certProvider \"service-ca\" }}\n")
	sb.WriteString("  annotations:\n")
	sb.WriteString("    service.beta.openshift.io/inject-cabundle: \"true\"\n")
	sb.WriteString("  {{- end }}\n")
}

// generateWebhookServices generates Service resources for webhook deployments.
func generateWebhookServices(b *bundle.RegistryV1) ([]byte, error) {
	webhookServicePortsByDeployment := map[string]sets.Set[corev1.ServicePort]{}
	for _, wh := range b.CSV.Spec.WebhookDefinitions {
		if _, ok := webhookServicePortsByDeployment[wh.DeploymentName]; !ok {
			webhookServicePortsByDeployment[wh.DeploymentName] = sets.New[corev1.ServicePort]()
		}
		webhookServicePortsByDeployment[wh.DeploymentName].Insert(getWebhookServicePort(wh))
	}

	var sb strings.Builder

	for _, depSpec := range b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		portSet, ok := webhookServicePortsByDeployment[depSpec.Name]
		if !ok {
			continue
		}

		sb.WriteString("---\n")

		svcName := serviceNameForDeployment(depSpec.Name)
		certName := certNameForDeployment(depSpec.Name)

		ports := portSet.UnsortedList()
		slices.SortStableFunc(ports, func(a, b corev1.ServicePort) int {
			return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.TargetPort.IntValue(), b.TargetPort.IntValue()))
		})

		sb.WriteString("apiVersion: v1\n")
		sb.WriteString("kind: Service\n")
		sb.WriteString("metadata:\n")
		fmt.Fprintf(&sb, "  name: %s\n", svcName)
		sb.WriteString("  namespace: {{ .Release.Namespace }}\n")

		// service-ca annotation for service
		sb.WriteString("  {{- if eq .Values.certProvider \"service-ca\" }}\n")
		sb.WriteString("  annotations:\n")
		fmt.Fprintf(&sb, "    service.beta.openshift.io/serving-cert-secret-name: %s\n", certName)
		sb.WriteString("  {{- end }}\n")

		sb.WriteString("spec:\n")

		// Selector from deployment spec
		if depSpec.Spec.Selector != nil && len(depSpec.Spec.Selector.MatchLabels) > 0 {
			sb.WriteString("  selector:\n")
			for k, v := range depSpec.Spec.Selector.MatchLabels {
				fmt.Fprintf(&sb, "    %s: %s\n", escapeHelm(k), escapeYAMLString(v))
			}
		}

		sb.WriteString("  ports:\n")
		for _, port := range ports {
			fmt.Fprintf(&sb, "  - name: \"%d\"\n", port.Port)
			fmt.Fprintf(&sb, "    port: %d\n", port.Port)
			fmt.Fprintf(&sb, "    targetPort: %s\n", port.TargetPort.String())
		}
	}

	return []byte(sb.String()), nil
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
