package helm

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
)

func generateDeployments(b *bundle.RegistryV1, webhookDeployments sets.Set[string]) ([]byte, error) {
	var sb strings.Builder
	first := true

	for _, depSpec := range b.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if !first {
			sb.WriteString("---\n")
		}
		first = false

		isWebhookDep := webhookDeployments.Has(depSpec.Name)
		if err := writeDeployment(&sb, b, depSpec, isWebhookDep); err != nil {
			return nil, err
		}
	}

	return []byte(sb.String()), nil
}

func writeDeployment(sb *strings.Builder, b *bundle.RegistryV1, depSpec v1alpha1.StrategyDeploymentSpec, isWebhookDep bool) error {
	certName := certNameForDeployment(depSpec.Name)

	// --- metadata ---
	sb.WriteString("apiVersion: apps/v1\n")
	sb.WriteString("kind: Deployment\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", depSpec.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")

	if len(depSpec.Label) > 0 {
		sb.WriteString("  labels:\n")
		labelsYAML, err := toYAMLIndent(map[string]string(depSpec.Label), 4)
		if err != nil {
			return fmt.Errorf("marshaling labels: %w", err)
		}
		sb.WriteString(labelsYAML + "\n")
	}

	// --- spec ---
	// Marshal the selector
	sb.WriteString("spec:\n")
	sb.WriteString("  revisionHistoryLimit: 1\n")

	if depSpec.Spec.Replicas != nil {
		sb.WriteString(fmt.Sprintf("  replicas: %d\n", *depSpec.Spec.Replicas))
	}

	if depSpec.Spec.Selector != nil {
		selectorYAML, err := toYAMLIndent(depSpec.Spec.Selector, 2)
		if err != nil {
			return fmt.Errorf("marshaling selector: %w", err)
		}
		sb.WriteString("  selector:\n")
		// The toYAMLIndent already adds 2 spaces, but selector content needs to be under "selector:"
		// Re-marshal with proper indent
		selectorData, err := yaml.Marshal(depSpec.Spec.Selector)
		if err != nil {
			return fmt.Errorf("marshaling selector: %w", err)
		}
		for _, line := range strings.Split(strings.TrimRight(string(selectorData), "\n"), "\n") {
			sb.WriteString("    " + escapeHelm(line) + "\n")
		}
		_ = selectorYAML
	}

	if depSpec.Spec.Strategy.Type != "" {
		strategyData, err := yaml.Marshal(depSpec.Spec.Strategy)
		if err != nil {
			return fmt.Errorf("marshaling strategy: %w", err)
		}
		sb.WriteString("  strategy:\n")
		for _, line := range strings.Split(strings.TrimRight(string(strategyData), "\n"), "\n") {
			sb.WriteString("    " + escapeHelm(line) + "\n")
		}
	}

	// --- template ---
	sb.WriteString("  template:\n")
	sb.WriteString("    metadata:\n")

	// Pod template labels
	if depSpec.Spec.Template.Labels != nil {
		sb.WriteString("      labels:\n")
		for k, v := range depSpec.Spec.Template.Labels {
			sb.WriteString(fmt.Sprintf("        %s: %s\n", escapeHelm(k), escapeYAMLString(v)))
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
		sb.WriteString(fmt.Sprintf("        %s: %s\n", escapeHelm(k), escapeYAMLString(v)))
		baseAnnoKeys = append(baseAnnoKeys, k)
	}
	sb.WriteString("        olm.targetNamespaces: {{ include \"olmTargetNamespaces\" . }}\n")

	// Config annotations — existing-wins merge
	if len(baseAnnoKeys) > 0 {
		sb.WriteString("        {{- if .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- range $k, $v := .Values.deploymentConfig.annotations }}\n")
		sb.WriteString("        {{- if not (hasKey (dict")
		for _, k := range baseAnnoKeys {
			sb.WriteString(fmt.Sprintf(" %q \"\"", k))
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
		sb.WriteString(fmt.Sprintf("      serviceAccountName: %s\n", depSpec.Spec.Template.Spec.ServiceAccountName))
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

	return nil
}

func writeNodeSelector(sb *strings.Builder, base map[string]string) {
	sb.WriteString("      {{- if .Values.deploymentConfig.nodeSelector }}\n")
	sb.WriteString("      nodeSelector: {{- toYaml .Values.deploymentConfig.nodeSelector | nindent 8 }}\n")
	sb.WriteString("      {{- else }}\n")
	if len(base) > 0 {
		sb.WriteString("      nodeSelector:\n")
		for k, v := range base {
			sb.WriteString(fmt.Sprintf("        %s: %s\n", escapeHelm(k), escapeYAMLString(v)))
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

	sb.WriteString(fmt.Sprintf("        {{- if %s }}\n", configPath))
	sb.WriteString(fmt.Sprintf("        %s: {{- toYaml %s | nindent 10 }}\n", field, configPath))
	if hasBase {
		sb.WriteString("        {{- else }}\n")
		baseData, _ := yaml.Marshal(baseValue)
		sb.WriteString(fmt.Sprintf("        %s:\n", field))
		for _, line := range strings.Split(strings.TrimRight(string(baseData), "\n"), "\n") {
			sb.WriteString("          " + escapeHelm(line) + "\n")
		}
	}
	sb.WriteString("        {{- end }}\n")
}

func writeTolerations(sb *strings.Builder, base []corev1.Toleration) {
	if len(base) > 0 {
		sb.WriteString("      tolerations:\n")
		baseData, _ := yaml.Marshal(base)
		for _, line := range strings.Split(strings.TrimRight(string(baseData), "\n"), "\n") {
			sb.WriteString("      " + escapeHelm(line) + "\n")
		}
		sb.WriteString("      {{- with .Values.deploymentConfig.tolerations }}\n")
		sb.WriteString("      {{- toYaml . | nindent 6 }}\n")
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
		baseData, _ := yaml.Marshal(base)
		for _, line := range strings.Split(strings.TrimRight(string(baseData), "\n"), "\n") {
			sb.WriteString("      " + escapeHelm(line) + "\n")
		}
	}

	// Cert volumes for webhook deployments
	if isWebhookDep {
		sb.WriteString("      {{- if ne .Values.certProvider \"\" }}\n")
		for _, cfg := range certVolumeConfigList {
			sb.WriteString(fmt.Sprintf("      - name: %s\n", cfg.Name))
			sb.WriteString("        secret:\n")
			sb.WriteString(fmt.Sprintf("          secretName: %s\n", certName))
			sb.WriteString("          items:\n")
			sb.WriteString("          - key: tls.crt\n")
			sb.WriteString(fmt.Sprintf("            path: %s\n", cfg.TLSCertPath))
			sb.WriteString("          - key: tls.key\n")
			sb.WriteString(fmt.Sprintf("            path: %s\n", cfg.TLSKeyPath))
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
	sb.WriteString(fmt.Sprintf("      - name: %s\n", c.Name))
	sb.WriteString(fmt.Sprintf("        image: %s\n", escapeYAMLString(c.Image)))

	if c.ImagePullPolicy != "" {
		sb.WriteString(fmt.Sprintf("        imagePullPolicy: %s\n", c.ImagePullPolicy))
	}

	if len(c.Command) > 0 {
		cmdData, _ := yaml.Marshal(c.Command)
		sb.WriteString("        command:\n")
		for _, line := range strings.Split(strings.TrimRight(string(cmdData), "\n"), "\n") {
			sb.WriteString("        " + escapeHelm(line) + "\n")
		}
	}

	if len(c.Args) > 0 {
		argsData, _ := yaml.Marshal(c.Args)
		sb.WriteString("        args:\n")
		for _, line := range strings.Split(strings.TrimRight(string(argsData), "\n"), "\n") {
			sb.WriteString("        " + escapeHelm(line) + "\n")
		}
	}

	if len(c.Ports) > 0 {
		portsData, _ := yaml.Marshal(c.Ports)
		sb.WriteString("        ports:\n")
		for _, line := range strings.Split(strings.TrimRight(string(portsData), "\n"), "\n") {
			sb.WriteString("        " + escapeHelm(line) + "\n")
		}
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
		probeData, _ := yaml.Marshal(c.LivenessProbe)
		sb.WriteString("        livenessProbe:\n")
		for _, line := range strings.Split(strings.TrimRight(string(probeData), "\n"), "\n") {
			sb.WriteString("          " + escapeHelm(line) + "\n")
		}
	}

	// ReadinessProbe
	if c.ReadinessProbe != nil {
		probeData, _ := yaml.Marshal(c.ReadinessProbe)
		sb.WriteString("        readinessProbe:\n")
		for _, line := range strings.Split(strings.TrimRight(string(probeData), "\n"), "\n") {
			sb.WriteString("          " + escapeHelm(line) + "\n")
		}
	}

	// SecurityContext
	if c.SecurityContext != nil {
		scData, _ := yaml.Marshal(c.SecurityContext)
		sb.WriteString("        securityContext:\n")
		for _, line := range strings.Split(strings.TrimRight(string(scData), "\n"), "\n") {
			sb.WriteString("          " + escapeHelm(line) + "\n")
		}
	}
}

func writeContainerEnv(sb *strings.Builder, baseEnv []corev1.EnvVar) {
	if len(baseEnv) > 0 {
		baseJSON, _ := json.Marshal(baseEnv)
		sb.WriteString(fmt.Sprintf("        env: {{- include \"mergeEnv\" (dict \"base\" `%s` \"overrides\" .Values.deploymentConfig.env) | nindent 8 }}\n", string(baseJSON)))
	} else {
		sb.WriteString("        {{- with .Values.deploymentConfig.env }}\n")
		sb.WriteString("        env: {{- toYaml . | nindent 8 }}\n")
		sb.WriteString("        {{- end }}\n")
	}
}

func writeContainerEnvFrom(sb *strings.Builder, baseEnvFrom []corev1.EnvFromSource) {
	if len(baseEnvFrom) > 0 {
		envFromData, _ := yaml.Marshal(baseEnvFrom)
		sb.WriteString("        envFrom:\n")
		for _, line := range strings.Split(strings.TrimRight(string(envFromData), "\n"), "\n") {
			sb.WriteString("        " + escapeHelm(line) + "\n")
		}
		sb.WriteString("        {{- with .Values.deploymentConfig.envFrom }}\n")
		sb.WriteString("        {{- toYaml . | nindent 8 }}\n")
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
		resData, _ := yaml.Marshal(c.Resources)
		sb.WriteString("        resources:\n")
		for _, line := range strings.Split(strings.TrimRight(string(resData), "\n"), "\n") {
			sb.WriteString("          " + escapeHelm(line) + "\n")
		}
	}
	sb.WriteString("        {{- end }}\n")
}

func writeContainerVolumeMounts(sb *strings.Builder, baseMounts []corev1.VolumeMount, isWebhookDep bool) {
	hasBase := len(baseMounts) > 0

	if hasBase {
		mountsData, _ := yaml.Marshal(baseMounts)
		sb.WriteString("        volumeMounts:\n")
		for _, line := range strings.Split(strings.TrimRight(string(mountsData), "\n"), "\n") {
			sb.WriteString("        " + escapeHelm(line) + "\n")
		}
	}

	// Cert volume mounts for webhook deployments
	if isWebhookDep {
		sb.WriteString("        {{- if ne .Values.certProvider \"\" }}\n")
		if !hasBase {
			sb.WriteString("        volumeMounts:\n")
		}
		for _, cfg := range certVolumeConfigList {
			sb.WriteString(fmt.Sprintf("        - name: %s\n", cfg.Name))
			sb.WriteString(fmt.Sprintf("          mountPath: %s\n", cfg.Path))
		}
		sb.WriteString("        {{- end }}\n")
	}

	// Config volume mounts — append
	sb.WriteString("        {{- with .Values.deploymentConfig.volumeMounts }}\n")
	if !hasBase && !isWebhookDep {
		sb.WriteString("        volumeMounts:\n")
	}
	sb.WriteString("        {{- toYaml . | nindent 8 }}\n")
	sb.WriteString("        {{- end }}\n")
}

