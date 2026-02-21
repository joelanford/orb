package helm

func generateHelpers() []byte {
	return []byte(`{{/*
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
}
