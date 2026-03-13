package helm

import (
	"strings"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func generateHelpers(supportedModes sets.Set[v1alpha1.InstallModeType]) []byte {
	var sb strings.Builder

	sb.WriteString(`{{/*
Compute olm.targetNamespaces annotation value.
If watchNamespace is empty (AllNamespaces), the value is empty string.
Otherwise, it is set to the watchNamespace value.
*/}}
{{- define "olmTargetNamespaces" -}}
{{- if eq .Values.watchNamespace "" -}}
""
{{- else -}}
{{ .Values.watchNamespace }}
{{- end -}}
{{- end -}}

{{/*
mergeEnv merges base env vars (passed as a JSON-encoded string) with
override env vars from .Values.deploymentConfig.env.
Override env vars take precedence over base env vars when names match.
New env vars from overrides are appended.
Usage: {{ include "mergeEnv" (dict "base" $baseJSON "overrides" .Values.deploymentConfig.env) }}
*/}}
{{- define "mergeEnv" -}}
{{- $base := list -}}
{{- if .base -}}
{{- $base = fromJson (printf "{\"l\":%s}" .base) -}}
{{- $base = $base.l -}}
{{- end -}}
{{- $overrides := default list .overrides -}}
{{- $overrideNames := dict -}}
{{- range $o := $overrides -}}
{{- $_ := set $overrideNames $o.name $o -}}
{{- end -}}
{{- $result := list -}}
{{- range $b := $base -}}
{{- if hasKey $overrideNames $b.name -}}
{{- $result = append $result (get $overrideNames $b.name) -}}
{{- $_ := unset $overrideNames $b.name -}}
{{- else -}}
{{- $result = append $result $b -}}
{{- end -}}
{{- end -}}
{{- range $o := $overrides -}}
{{- if hasKey $overrideNames $o.name -}}
{{- $result = append $result $o -}}
{{- $_ := unset $overrideNames $o.name -}}
{{- end -}}
{{- end -}}
{{- toYaml $result -}}
{{- end -}}
`)

	hasAllNS := supportedModes.Has(v1alpha1.InstallModeTypeAllNamespaces)
	hasOwn := supportedModes.Has(v1alpha1.InstallModeTypeOwnNamespace)
	hasSingle := supportedModes.Has(v1alpha1.InstallModeTypeSingleNamespace)

	switch {
	case !hasAllNS && hasOwn && !hasSingle:
		sb.WriteString(`
{{/*
Validate watchNamespace for OwnNamespace install mode.
The operator only supports OwnNamespace mode, so watchNamespace
must equal the release namespace.
*/}}
{{- define "validateWatchNamespace" -}}
{{- if and .Values.watchNamespace (ne .Values.watchNamespace .Release.Namespace) -}}
{{- fail "watchNamespace must equal the release namespace (operator only supports OwnNamespace install mode)" -}}
{{- end -}}
{{- end -}}
`)
	case !hasAllNS && hasSingle && !hasOwn:
		sb.WriteString(`
{{/*
Validate watchNamespace for SingleNamespace install mode.
The operator only supports SingleNamespace mode, so watchNamespace
must differ from the release namespace.
*/}}
{{- define "validateWatchNamespace" -}}
{{- if and .Values.watchNamespace (eq .Values.watchNamespace .Release.Namespace) -}}
{{- fail "watchNamespace must differ from the release namespace (operator only supports SingleNamespace install mode)" -}}
{{- end -}}
{{- end -}}
`)
	default:
		sb.WriteString(`
{{- define "validateWatchNamespace" -}}
{{- end -}}
`)
	}

	return []byte(sb.String())
}
