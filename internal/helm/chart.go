package helm

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
)

// Generate produces a Helm *chart.Chart from a registry+v1 bundle.
func Generate(b *bundle.RegistryV1) (*chart.Chart, error) {
	if err := convert.Converter.BundleValidator.Validate(b); err != nil {
		return nil, fmt.Errorf("bundle validation failed: %w", err)
	}

	hasWebhooks := len(b.CSV.Spec.WebhookDefinitions) > 0

	supportedInstallModes := sets.New[v1alpha1.InstallModeType]()
	for _, im := range b.CSV.Spec.InstallModes {
		if im.Supported {
			supportedInstallModes.Insert(im.Type)
		}
	}

	// Collect webhook deployments
	webhookDeployments := sets.New[string]()
	for _, wh := range b.CSV.Spec.WebhookDefinitions {
		webhookDeployments.Insert(wh.DeploymentName)
	}

	// Chart metadata
	md := &chart.Metadata{
		APIVersion:  chart.APIVersionV2,
		Name:        b.PackageName,
		Version:     b.CSV.Spec.Version.String(),
		Description: cmp.Or(b.CSV.Spec.Description, b.CSV.Spec.DisplayName, b.PackageName),
		Type:        "application",
	}
	if len(b.CSV.Spec.Icon) > 0 && b.CSV.Spec.Icon[0].Data != "" && b.CSV.Spec.Icon[0].MediaType != "" {
		icon := b.CSV.Spec.Icon[0]
		md.Icon = fmt.Sprintf("data:%s;base64,%s", icon.MediaType, icon.Data)
	}

	// Values
	valuesRaw, values, err := generateValues(hasWebhooks)
	if err != nil {
		return nil, fmt.Errorf("generating values.yaml: %w", err)
	}

	// Schema
	schemaData, err := generateSchema(b, supportedInstallModes, hasWebhooks)
	if err != nil {
		return nil, fmt.Errorf("generating values.schema.json: %w", err)
	}

	// Templates
	templates := []*common.File{{
		Name: "templates/_helpers.tpl",
		Data: generateHelpers(),
	}}

	type templateGen struct {
		name     string
		generate func() ([]byte, error)
	}

	generators := []templateGen{
		{"templates/serviceaccount.yaml", func() ([]byte, error) { return generateServiceAccounts(b) }},
		{"templates/clusterrole.yaml", func() ([]byte, error) { return generateRBAC(b) }},
		{"templates/deployment.yaml", func() ([]byte, error) { return generateDeployments(b, webhookDeployments) }},
		{"templates/crd.yaml", func() ([]byte, error) { return generateCRDs(b) }},
		{"templates/additional.yaml", func() ([]byte, error) { return generateAdditional(b) }},
	}
	if hasWebhooks {
		generators = append(generators,
			templateGen{"templates/webhook.yaml", func() ([]byte, error) { return generateWebhooks(b) }},
			templateGen{"templates/service.yaml", func() ([]byte, error) { return generateWebhookServices(b) }},
			templateGen{"templates/cert-manager.yaml", func() ([]byte, error) { return generateCertProvider(b) }},
		)
	}

	for _, g := range generators {
		data, err := g.generate()
		if err != nil {
			return nil, fmt.Errorf("generating %s: %w", g.name, err)
		}
		if len(data) > 0 {
			templates = append(templates, &common.File{Name: g.name, Data: data})
		}
	}

	return &chart.Chart{
		Metadata:  md,
		Raw:       []*common.File{{Name: "values.yaml", Data: valuesRaw}},
		Values:    values,
		Schema:    schemaData,
		Templates: templates,
	}, nil
}

// certVolumeConfig matches the render package's unexported certVolumeConfig.
type certVolumeConfig struct {
	Name        string
	Path        string
	TLSCertPath string
	TLSKeyPath  string
}

var certVolumeConfigList = []certVolumeConfig{
	{
		Name:        "webhook-cert",
		Path:        "/tmp/k8s-webhook-server/serving-certs",
		TLSCertPath: "tls.crt",
		TLSKeyPath:  "tls.key",
	},
	{
		Name:        "apiservice-cert",
		Path:        "/apiserver.local.config/certificates",
		TLSCertPath: "apiserver.crt",
		TLSKeyPath:  "apiserver.key",
	},
}

// escapeHelm escapes {{ in strings so Helm doesn't interpret them.
// It uses backtick-quoted raw strings ({{ `{{` }}) rather than double-quoted
// strings ({{ "{{" }}) because the latter breaks when the value ends up inside
// a YAML double-quoted string — YAML escaping turns " into \", producing
// {{ \"{{\" }} which is invalid Go template syntax. Backticks are not special
// in YAML, so they survive any YAML quoting context unchanged.
func escapeHelm(s string) string {
	return strings.ReplaceAll(s, "{{", "{{ `{{` }}")
}

// writeYAMLField marshals obj to YAML, writes "fieldName:" at the given indent,
// then writes the marshaled value indented by (indent+2) spaces on subsequent lines.
// All lines are escaped for Helm template syntax.
func writeYAMLField(sb *strings.Builder, fieldName string, indent int, obj interface{}) {
	fmt.Fprintf(sb, "%s%s:\n", strings.Repeat(" ", indent), fieldName)
	writeYAMLFieldRaw(sb, indent+2, obj)
}

// writeYAMLFieldRaw marshals obj to YAML and writes each line at the given indent,
// without a preceding field label. All lines are escaped for Helm template syntax.
func writeYAMLFieldRaw(sb *strings.Builder, indent int, obj interface{}) {
	data, err := yaml.Marshal(obj)
	if err != nil {
		// All callers pass well-known Kubernetes API types that always marshal
		// successfully, so a failure here indicates a programming error.
		panic(fmt.Sprintf("yaml.Marshal: %v", err))
	}
	prefix := strings.Repeat(" ", indent)
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		sb.WriteString(prefix + escapeHelm(line) + "\n")
	}
}

// escapeYAMLString returns a YAML-safe representation of a string value,
// scanning for problematic characters and quoting if necessary.
func escapeYAMLString(s string) string {
	s = escapeHelm(s)
	if strings.ContainsAny(s, ":\n\"'{}[]|>&*!%#@`,?") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// certNameForDeployment returns the cert secret name for a given deployment name,
// using the same logic as the render package.
func certNameForDeployment(depName string) string {
	webhookServiceName := convert.ObjectNameForBaseAndSuffix(strings.ReplaceAll(depName, ".", "-"), "service")
	return convert.ObjectNameForBaseAndSuffix(webhookServiceName, "cert")
}

// serviceNameForDeployment returns the webhook service name for a given deployment name.
func serviceNameForDeployment(depName string) string {
	return convert.ObjectNameForBaseAndSuffix(strings.ReplaceAll(depName, ".", "-"), "service")
}

// saNameOrDefault returns "default" when saName is empty.
func saNameOrDefault(saName string) string {
	return cmp.Or(saName, "default")
}
