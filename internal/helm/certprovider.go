package helm

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
)

func generateCertProvider(b *bundle.RegistryV1) ([]byte, error) {
	webhookDeployments := sets.New[string]()
	for _, wh := range b.CSV.Spec.WebhookDefinitions {
		webhookDeployments.Insert(wh.DeploymentName)
	}

	if webhookDeployments.Len() == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("{{- if eq .Values.certProvider \"cert-manager\" }}\n")

	first := true
	for _, depName := range webhookDeployments.UnsortedList() {
		svcName := serviceNameForDeployment(depName)
		certName := certNameForDeployment(depName)
		issuerName := convert.ObjectNameForBaseAndSuffix(certName, "selfsigned-issuer")

		if !first {
			sb.WriteString("---\n")
		}
		first = false

		// Issuer
		sb.WriteString(fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: %s
  namespace: {{ .Release.Namespace }}
spec:
  selfSigned: {}
`, issuerName))

		sb.WriteString("---\n")

		// Certificate
		sb.WriteString(fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: %s
  namespace: {{ .Release.Namespace }}
spec:
  secretName: %s
  usages:
  - server auth
  isCA: false
  dnsNames:
  - %s.{{ .Release.Namespace }}
  - %s.{{ .Release.Namespace }}.svc
  - %s.{{ .Release.Namespace }}.svc.cluster.local
  issuerRef:
    name: %s
  duration: 17520h0m0s
  renewBefore: 24h0m0s
`, certName, certName, svcName, svcName, svcName, issuerName))
	}

	sb.WriteString("{{- end }}\n")

	return []byte(sb.String()), nil
}
