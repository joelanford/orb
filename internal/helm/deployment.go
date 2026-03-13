package helm

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
)

func generateDeployments(b *bundle.RegistryV1, webhookDeployments sets.Set[string]) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("{{- include \"validateWatchNamespace\" . -}}\n")
	for _, depSpec := range b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		sb.WriteString("---\n")

		isWebhookDep := webhookDeployments.Has(depSpec.Name)
		writeDeployment(&sb, b, depSpec, isWebhookDep)
	}

	return []byte(sb.String()), nil
}

func writeDeployment(sb *strings.Builder, b *bundle.RegistryV1, depSpec v1alpha1.StrategyDeploymentSpec, isWebhookDep bool) {
	certName := certNameForDeployment(depSpec.Name)

	// --- metadata ---
	sb.WriteString("apiVersion: apps/v1\n")
	sb.WriteString("kind: Deployment\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(sb, "  name: %s\n", depSpec.Name)
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")

	if len(depSpec.Label) > 0 {
		writeYAMLField(sb, "labels", 2, map[string]string(depSpec.Label))
	}

	// --- spec ---
	// Marshal the selector
	sb.WriteString("spec:\n")
	sb.WriteString("  revisionHistoryLimit: 1\n")

	if depSpec.Spec.Replicas != nil {
		fmt.Fprintf(sb, "  replicas: %d\n", *depSpec.Spec.Replicas)
	}

	if depSpec.Spec.Selector != nil {
		writeYAMLField(sb, "selector", 2, depSpec.Spec.Selector)
	}

	if depSpec.Spec.Strategy.Type != "" {
		writeYAMLField(sb, "strategy", 2, depSpec.Spec.Strategy)
	}

	// --- template ---
	sb.WriteString("  template:\n")
	sb.WriteString("    metadata:\n")

	// Pod template labels
	if depSpec.Spec.Template.Labels != nil {
		sb.WriteString("      labels:\n")
		for k, v := range depSpec.Spec.Template.Labels {
			fmt.Fprintf(sb, "        %s: %s\n", escapeHelm(k), escapeYAMLString(v))
		}
	}

	// Pod template annotations — merge CSV annotations + template annotations + olm.targetNamespaces
	mergedAnnotations := convert.MergeMaps(b.CSV.Annotations, depSpec.Spec.Template.Annotations)
	sb.WriteString("      annotations:\n")
	baseAnnoKeys := make([]string, 0, len(mergedAnnotations))
	for k, v := range mergedAnnotations {
		if k == "olm.targetNamespaces" {
			continue
		}
		fmt.Fprintf(sb, "        %s: %s\n", escapeHelm(k), escapeYAMLString(v))
		baseAnnoKeys = append(baseAnnoKeys, k)
	}
	sb.WriteString("        olm.targetNamespaces: {{ include \"olmTargetNamespaces\" . }}\n")

	// Config annotations — existing-wins merge
	if len(baseAnnoKeys) > 0 {
		sb.WriteString("        {{- if .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- range $k, $v := .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- if not (hasKey (dict")
		for _, k := range baseAnnoKeys {
			fmt.Fprintf(sb, " %q \"\"", k)
		}
		sb.WriteString(" \"olm.targetNamespaces\" \"\") $k) }}\n")
		sb.WriteString("        {{ $k }}: {{ $v }}\n")
		sb.WriteString("        {{- end }}\n")
		sb.WriteString("        {{- end }}\n")
		sb.WriteString("        {{- end }}\n")
	} else {
		sb.WriteString("        {{- if .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- range $k, $v := .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- if ne $k \"olm.targetNamespaces\" }}\n")
		sb.WriteString("        {{ $k }}: {{ $v }}\n")
		sb.WriteString("        {{- end }}\n")
		sb.WriteString("        {{- end }}\n")
		sb.WriteString("        {{- end }}\n")
	}

	// --- pod spec ---
	sb.WriteString("    spec:\n")

	// ServiceAccountName
	if depSpec.Spec.Template.Spec.ServiceAccountName != "" {
		fmt.Fprintf(sb, "      serviceAccountName: %s\n", depSpec.Spec.Template.Spec.ServiceAccountName)
	}

	// NodeSelector — replace semantics
	writeNodeSelector(sb, depSpec.Spec.Template.Spec.NodeSelector)

	// Affinity — selective override
	writeAffinity(sb, depSpec.Spec.Template.Spec.Affinity)

	// Tolerations — append semantics
	writeTolerations(sb, depSpec.Spec.Template.Spec.Tolerations)

	// Volumes — append semantics + conditional cert volumes
	writeVolumes(sb, depSpec.Spec.Template.Spec.Volumes, isWebhookDep, certName)

	// Containers
	writeContainers(sb, depSpec.Spec.Template.Spec.Containers, isWebhookDep, certName)

	// InitContainers (if any)
	if len(depSpec.Spec.Template.Spec.InitContainers) > 0 {
		sb.WriteString("      initContainers:\n")
		for _, c := range depSpec.Spec.Template.Spec.InitContainers {
			writeContainerBase(sb, c, false, "")
		}
	}

}

func writeNodeSelector(sb *strings.Builder, base map[string]string) {
	sb.WriteString("      {{- if .Values.deploymentConfig.nodeSelector }}\n")
	sb.WriteString("      nodeSelector: {{- toYaml .Values.deploymentConfig.nodeSelector | nindent 8 }}\n")
	sb.WriteString("      {{- else }}\n")
	if len(base) > 0 {
		sb.WriteString("      nodeSelector:\n")
		for k, v := range base {
			fmt.Fprintf(sb, "        %s: %s\n", escapeHelm(k), escapeYAMLString(v))
		}
	}
	sb.WriteString("      {{- end }}\n")
}

func writeAffinity(sb *strings.Builder, base *corev1.Affinity) {
	if base == nil {
		// No base affinity — just use config if present
		sb.WriteString("      {{- with .Values.deploymentConfig.affinity }}\n")
		sb.WriteString("      affinity: {{- toYaml . | nindent 8 }}\n")
		sb.WriteString("      {{- end }}\n")
		return
	}

	sb.WriteString("      affinity:\n")

	// NodeAffinity
	writeAffinitySubField(sb, "nodeAffinity", base.NodeAffinity)
	writeAffinitySubField(sb, "podAffinity", base.PodAffinity)
	writeAffinitySubField(sb, "podAntiAffinity", base.PodAntiAffinity)
}

func writeAffinitySubField(sb *strings.Builder, field string, baseValue interface{}) {
	configPath := fmt.Sprintf(".Values.deploymentConfig.affinity.%s", field)

	hasBase := false
	switch v := baseValue.(type) {
	case *corev1.NodeAffinity:
		hasBase = v != nil
	case *corev1.PodAffinity:
		hasBase = v != nil
	case *corev1.PodAntiAffinity:
		hasBase = v != nil
	}

	fmt.Fprintf(sb, "        {{- if and .Values.deploymentConfig.affinity %s }}\n", configPath)
	fmt.Fprintf(sb, "        %s: {{- toYaml %s | nindent 10 }}\n", field, configPath)
	if hasBase {
		sb.WriteString("        {{- else }}\n")
		writeYAMLField(sb, field, 8, baseValue)
	}
	sb.WriteString("        {{- end }}\n")
}

func writeTolerations(sb *strings.Builder, base []corev1.Toleration) {
	if len(base) > 0 {
		writeYAMLField(sb, "tolerations", 6, base)
		sb.WriteString("      {{- with .Values.deploymentConfig.tolerations }}\n")
		sb.WriteString("      {{- toYaml . | nindent 8 }}\n")
		sb.WriteString("      {{- end }}\n")
	} else {
		sb.WriteString("      {{- with .Values.deploymentConfig.tolerations }}\n")
		sb.WriteString("      tolerations:\n")
		sb.WriteString("      {{- toYaml . | nindent 6 }}\n")
		sb.WriteString("      {{- end }}\n")
	}
}

func writeVolumes(sb *strings.Builder, base []corev1.Volume, isWebhookDep bool, certName string) {
	hasBaseVolumes := len(base) > 0

	if hasBaseVolumes || isWebhookDep {
		sb.WriteString("      volumes:\n")
	}

	if hasBaseVolumes {
		writeYAMLFieldRaw(sb, 6, base)
	}

	// Cert volumes for webhook deployments
	if isWebhookDep {
		sb.WriteString("      {{- if ne .Values.certProvider \"\" }}\n")
		for _, cfg := range certVolumeConfigList {
			fmt.Fprintf(sb, "      - name: %s\n", cfg.Name)
			sb.WriteString("        secret:\n")
			fmt.Fprintf(sb, "          secretName: %s\n", certName)
			sb.WriteString("          items:\n")
			sb.WriteString("          - key: tls.crt\n")
			fmt.Fprintf(sb, "            path: %s\n", cfg.TLSCertPath)
			sb.WriteString("          - key: tls.key\n")
			fmt.Fprintf(sb, "            path: %s\n", cfg.TLSKeyPath)
		}
		sb.WriteString("      {{- end }}\n")
	}

	// Config volumes — append
	sb.WriteString("      {{- with .Values.deploymentConfig.volumes }}\n")
	if !hasBaseVolumes && !isWebhookDep {
		sb.WriteString("      volumes:\n")
	}
	sb.WriteString("      {{- toYaml . | nindent 6 }}\n")
	sb.WriteString("      {{- end }}\n")
}

func writeContainers(sb *strings.Builder, containers []corev1.Container, isWebhookDep bool, certName string) {
	sb.WriteString("      containers:\n")
	for _, c := range containers {
		writeContainerBase(sb, c, isWebhookDep, certName)
	}
}

func writeContainerBase(sb *strings.Builder, c corev1.Container, isWebhookDep bool, _ string) {
	fmt.Fprintf(sb, "      - name: %s\n", c.Name)
	fmt.Fprintf(sb, "        image: %s\n", escapeYAMLString(c.Image))

	if c.ImagePullPolicy != "" {
		fmt.Fprintf(sb, "        imagePullPolicy: %s\n", c.ImagePullPolicy)
	}

	if len(c.Command) > 0 {
		writeYAMLField(sb, "command", 8, c.Command)
	}

	if len(c.Args) > 0 {
		writeYAMLField(sb, "args", 8, c.Args)
	}

	if len(c.Ports) > 0 {
		writeYAMLField(sb, "ports", 8, c.Ports)
	}

	// Env — mergeEnv semantics
	writeContainerEnv(sb, c.Env)

	// EnvFrom — append
	writeContainerEnvFrom(sb, c.EnvFrom)

	// Resources — replace
	writeContainerResources(sb, &c)

	// VolumeMounts — append + cert mounts
	writeContainerVolumeMounts(sb, c.VolumeMounts, isWebhookDep)

	// LivenessProbe
	if c.LivenessProbe != nil {
		writeYAMLField(sb, "livenessProbe", 8, c.LivenessProbe)
	}

	// ReadinessProbe
	if c.ReadinessProbe != nil {
		writeYAMLField(sb, "readinessProbe", 8, c.ReadinessProbe)
	}

	// SecurityContext
	if c.SecurityContext != nil {
		writeYAMLField(sb, "securityContext", 8, c.SecurityContext)
	}
}

func writeContainerEnv(sb *strings.Builder, baseEnv []corev1.EnvVar) {
	if len(baseEnv) > 0 {
		baseJSON, _ := json.Marshal(baseEnv)
		fmt.Fprintf(sb, "        env: {{- include \"mergeEnv\" (dict \"base\" `%s` \"overrides\" .Values.deploymentConfig.env) | nindent 8 }}\n", string(baseJSON))
	} else {
		sb.WriteString("        {{- with .Values.deploymentConfig.env }}\n")
		sb.WriteString("        env: {{- toYaml . | nindent 8 }}\n")
		sb.WriteString("        {{- end }}\n")
	}
}

func writeContainerEnvFrom(sb *strings.Builder, baseEnvFrom []corev1.EnvFromSource) {
	if len(baseEnvFrom) > 0 {
		writeYAMLField(sb, "envFrom", 8, baseEnvFrom)
		sb.WriteString("        {{- with .Values.deploymentConfig.envFrom }}\n")
		sb.WriteString("        {{- toYaml . | nindent 10 }}\n")
		sb.WriteString("        {{- end }}\n")
	} else {
		sb.WriteString("        {{- with .Values.deploymentConfig.envFrom }}\n")
		sb.WriteString("        envFrom:\n")
		sb.WriteString("        {{- toYaml . | nindent 8 }}\n")
		sb.WriteString("        {{- end }}\n")
	}
}

func writeContainerResources(sb *strings.Builder, c *corev1.Container) {
	sb.WriteString("        {{- if .Values.deploymentConfig.resources }}\n")
	sb.WriteString("        resources: {{- toYaml .Values.deploymentConfig.resources | nindent 10 }}\n")
	sb.WriteString("        {{- else }}\n")
	if c.Resources.Limits != nil || c.Resources.Requests != nil {
		writeYAMLField(sb, "resources", 8, c.Resources)
	}
	sb.WriteString("        {{- end }}\n")
}

func writeContainerVolumeMounts(sb *strings.Builder, baseMounts []corev1.VolumeMount, isWebhookDep bool) {
	hasBase := len(baseMounts) > 0

	if hasBase {
		writeYAMLField(sb, "volumeMounts", 8, baseMounts)
	}

	// Cert volume mounts for webhook deployments
	if isWebhookDep {
		sb.WriteString("        {{- if ne .Values.certProvider \"\" }}\n")
		if !hasBase {
			sb.WriteString("        volumeMounts:\n")
		}
		for _, cfg := range certVolumeConfigList {
			fmt.Fprintf(sb, "          - name: %s\n", cfg.Name)
			fmt.Fprintf(sb, "            mountPath: %s\n", cfg.Path)
		}
		sb.WriteString("        {{- end }}\n")
	}

	// Config volume mounts — append
	sb.WriteString("        {{- with .Values.deploymentConfig.volumeMounts }}\n")
	if !hasBase && !isWebhookDep {
		sb.WriteString("        volumeMounts:\n")
	}
	sb.WriteString("        {{- toYaml . | nindent 10 }}\n")
	sb.WriteString("        {{- end }}\n")
}
