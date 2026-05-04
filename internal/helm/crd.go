package helm

import (
	"fmt"
	"strings"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func generateCRDs(b *registryv1.Bundle) ([]byte, error) {
	if len(b.CRDs) == 0 {
		return nil, nil
	}

	// Build map of CRD name -> conversion webhook description
	crdToWebhook := map[string]v1alpha1.WebhookDescription{}
	for _, wh := range b.CSV.Spec.WebhookDefinitions {
		if wh.Type != v1alpha1.ConversionWebhook {
			continue
		}
		for _, crdName := range wh.ConversionCRDs {
			crdToWebhook[crdName] = wh
		}
	}

	var sb strings.Builder

	for _, crd := range b.CRDs {
		sb.WriteString("---\n")

		cp := crd.DeepCopy()

		wh, hasConversion := crdToWebhook[crd.Name]
		var certName string
		if hasConversion {
			certName = certNameForDeployment(wh.DeploymentName)
			svcName := serviceNameForDeployment(wh.DeploymentName)

			conversionWebhookPath := "/"
			if wh.WebhookPath != nil {
				conversionWebhookPath = *wh.WebhookPath
			}

			cp.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ClientConfig: &apiextensionsv1.WebhookClientConfig{
						Service: &apiextensionsv1.ServiceReference{
							Namespace: "HELM_RELEASE_NS",
							Name:      svcName,
							Path:      &conversionWebhookPath,
							Port:      &wh.ContainerPort,
						},
					},
					ConversionReviewVersions: wh.AdmissionReviewVersions,
				},
			}
		}

		// Marshal the full CRD
		crdData, err := yaml.Marshal(cp)
		if err != nil {
			return nil, fmt.Errorf("marshaling CRD %s: %w", crd.Name, err)
		}

		crdYAML := string(crdData)

		// Replace namespace placeholder with Helm template
		if hasConversion {
			crdYAML = strings.ReplaceAll(crdYAML, "HELM_RELEASE_NS", "{{ .Release.Namespace }}")
		}

		// Escape any literal {{ in the CRD YAML (e.g., in annotations), but not our template directives
		crdYAML = escapeHelmExceptDirectives(crdYAML)

		// For CRDs with conversion webhooks, add conditional cert-provider annotations
		if hasConversion {
			// Insert cert annotations after the metadata block
			crdYAML = insertCRDCertAnnotations(crdYAML, certName)
		}

		sb.WriteString(crdYAML)
	}

	return []byte(sb.String()), nil
}

// insertCRDCertAnnotations adds conditional cert-provider annotation blocks
// after the existing metadata in the YAML.
func insertCRDCertAnnotations(crdYAML string, certName string) string {
	// We need to add annotations to the metadata section.
	// The approach: find "metadata:" line and add annotations after any existing metadata content.
	// Since the CRD may already have annotations, we append conditionals after the full YAML.
	// A simpler approach: wrap the entire CRD in a conditional that adds annotations.

	var sb strings.Builder
	sb.WriteString("{{- if eq .Values.certProvider \"cert-manager\" }}\n")

	// Re-emit the CRD YAML but with cert-manager annotation injected into metadata
	sb.WriteString(injectMetadataAnnotation(crdYAML,
		fmt.Sprintf("cert-manager.io/inject-ca-from: {{ .Release.Namespace }}/%s", certName)))

	sb.WriteString("{{- else if eq .Values.certProvider \"service-ca\" }}\n")

	sb.WriteString(injectMetadataAnnotation(crdYAML,
		`service.beta.openshift.io/inject-cabundle: "true"`))

	sb.WriteString("{{- else }}\n")
	sb.WriteString(crdYAML)
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// injectMetadataAnnotation takes a YAML string and injects an annotation into the metadata section.
func injectMetadataAnnotation(yamlStr string, annotation string) string {
	lines := strings.Split(yamlStr, "\n")
	var sb strings.Builder

	foundMetadata := false
	injected := false

	for _, line := range lines {
		if !foundMetadata && strings.TrimSpace(line) == "metadata:" {
			foundMetadata = true
			sb.WriteString(line + "\n")
			continue
		}

		// After metadata:, look for the annotations: key or inject before the next top-level key
		if foundMetadata && !injected {
			trimmed := strings.TrimSpace(line)
			if trimmed == "annotations:" {
				// There are already annotations — add ours after existing ones
				sb.WriteString(line + "\n")
				sb.WriteString("    " + annotation + "\n")
				injected = true
				continue
			}
			// If we hit a non-indented line (next top-level key under metadata) or a line with
			// 2-space indent that starts a new metadata field, inject annotations first
			if len(line) > 0 && !strings.HasPrefix(line, "  ") {
				// This is a top-level key like "spec:", inject before it
				sb.WriteString("  annotations:\n")
				sb.WriteString("    " + annotation + "\n")
				injected = true
				sb.WriteString(line + "\n")
				continue
			}
		}

		sb.WriteString(line + "\n")
	}

	return sb.String()
}

// escapeHelmExceptDirectives escapes {{ in YAML except for lines containing {{ .Release or {{ .Values
// or other Helm template directives we've already inserted.
func escapeHelmExceptDirectives(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.Contains(line, "{{") {
			// Check if this looks like one of our template directives
			if strings.Contains(line, "{{ .Release.") || strings.Contains(line, "{{ .Values.") ||
				strings.Contains(line, "{{-") || strings.Contains(line, "{{ include") ||
				strings.Contains(line, "{{ toYaml") {
				continue
			}
			lines[i] = escapeHelm(line)
		}
	}
	return strings.Join(lines, "\n")
}
