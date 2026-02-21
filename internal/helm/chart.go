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
	var templates []*common.File

	templates = append(templates, &common.File{
		Name: "templates/_helpers.tpl",
		Data: generateHelpers(),
	})

	saData, err := generateServiceAccounts(b)
	if err != nil {
		return nil, fmt.Errorf("generating serviceaccount templates: %w", err)
	}
	if len(saData) > 0 {
		templates = append(templates, &common.File{Name: "templates/serviceaccount.yaml", Data: saData})
	}

	rbacData, err := generateRBAC(b)
	if err != nil {
		return nil, fmt.Errorf("generating rbac templates: %w", err)
	}
	if len(rbacData) > 0 {
		templates = append(templates, &common.File{Name: "templates/clusterrole.yaml", Data: rbacData})
	}

	depData, err := generateDeployments(b, webhookDeployments)
	if err != nil {
		return nil, fmt.Errorf("generating deployment templates: %w", err)
	}
	if len(depData) > 0 {
		templates = append(templates, &common.File{Name: "templates/deployment.yaml", Data: depData})
	}

	crdData, err := generateCRDs(b)
	if err != nil {
		return nil, fmt.Errorf("generating crd templates: %w", err)
	}
	if len(crdData) > 0 {
		templates = append(templates, &common.File{Name: "templates/crd.yaml", Data: crdData})
	}

	if hasWebhooks {
		whData, err := generateWebhooks(b)
		if err != nil {
			return nil, fmt.Errorf("generating webhook templates: %w", err)
		}
		if len(whData) > 0 {
			templates = append(templates, &common.File{Name: "templates/webhook.yaml", Data: whData})
		}

		svcData, err := generateWebhookServices(b)
		if err != nil {
			return nil, fmt.Errorf("generating service templates: %w", err)
		}
		if len(svcData) > 0 {
			templates = append(templates, &common.File{Name: "templates/service.yaml", Data: svcData})
		}

		certData, err := generateCertProvider(b)
		if err != nil {
			return nil, fmt.Errorf("generating cert-manager templates: %w", err)
		}
		if len(certData) > 0 {
			templates = append(templates, &common.File{Name: "templates/cert-manager.yaml", Data: certData})
		}
	}

	addlData, err := generateAdditional(b)
	if err != nil {
		return nil, fmt.Errorf("generating additional templates: %w", err)
	}
	if len(addlData) > 0 {
		templates = append(templates, &common.File{Name: "templates/additional.yaml", Data: addlData})
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

// toYAMLIndent marshals obj to YAML and indents each line by the given number of spaces.
func toYAMLIndent(obj interface{}, indent int) (string, error) {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	s := strings.TrimRight(string(data), "\n")
	if indent <= 0 {
		return s, nil
	}
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n"), nil
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
