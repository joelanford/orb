package helm

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/render"
)

// Generate produces a Helm chart as a map of file paths to file contents
// from a registry+v1 bundle.
func Generate(b *bundle.RegistryV1) (map[string][]byte, error) {
	if err := render.Renderer.BundleValidator.Validate(b); err != nil {
		return nil, fmt.Errorf("bundle validation failed: %w", err)
	}

	chartDir := b.PackageName

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

	files := map[string][]byte{}

	// Chart.yaml
	chartYAML := map[string]interface{}{
		"apiVersion":  "v2",
		"name":        b.PackageName,
		"version":     b.CSV.Spec.Version.String(),
		"description": cmp.Or(b.CSV.Spec.Description, b.CSV.Spec.DisplayName, b.PackageName),
		"type":        "application",
	}
	if len(b.CSV.Spec.Icon) > 0 && b.CSV.Spec.Icon[0].Data != "" && b.CSV.Spec.Icon[0].MediaType != "" {
		icon := b.CSV.Spec.Icon[0]
		chartYAML["icon"] = fmt.Sprintf("data:%s;base64,%s", icon.MediaType, icon.Data)
	}
	chartData, err := yaml.Marshal(chartYAML)
	if err != nil {
		return nil, fmt.Errorf("marshaling Chart.yaml: %w", err)
	}
	files[chartDir+"/Chart.yaml"] = chartData

	// values.yaml
	values := map[string]interface{}{
		"watchNamespace":   "",
		"deploymentConfig": map[string]interface{}{},
	}
	if hasWebhooks {
		values["certProvider"] = "cert-manager"
	}
	valuesData, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshaling values.yaml: %w", err)
	}
	files[chartDir+"/values.yaml"] = valuesData

	// values.schema.json
	schemaData, err := generateSchema(b, supportedInstallModes, hasWebhooks)
	if err != nil {
		return nil, fmt.Errorf("generating values.schema.json: %w", err)
	}
	files[chartDir+"/values.schema.json"] = schemaData

	// _helpers.tpl
	files[chartDir+"/templates/_helpers.tpl"] = generateHelpers()

	// ServiceAccount templates
	saData, err := generateServiceAccounts(b)
	if err != nil {
		return nil, fmt.Errorf("generating serviceaccount templates: %w", err)
	}
	if len(saData) > 0 {
		files[chartDir+"/templates/serviceaccount.yaml"] = saData
	}

	// RBAC templates
	rbacData, err := generateRBAC(b)
	if err != nil {
		return nil, fmt.Errorf("generating rbac templates: %w", err)
	}
	if len(rbacData) > 0 {
		files[chartDir+"/templates/clusterrole.yaml"] = rbacData
	}

	// Deployment templates
	depData, err := generateDeployments(b, webhookDeployments)
	if err != nil {
		return nil, fmt.Errorf("generating deployment templates: %w", err)
	}
	if len(depData) > 0 {
		files[chartDir+"/templates/deployment.yaml"] = depData
	}

	// CRD templates
	crdData, err := generateCRDs(b)
	if err != nil {
		return nil, fmt.Errorf("generating crd templates: %w", err)
	}
	if len(crdData) > 0 {
		files[chartDir+"/templates/crd.yaml"] = crdData
	}

	// Webhook + Service templates
	if hasWebhooks {
		whData, err := generateWebhooks(b)
		if err != nil {
			return nil, fmt.Errorf("generating webhook templates: %w", err)
		}
		if len(whData) > 0 {
			files[chartDir+"/templates/webhook.yaml"] = whData
		}

		svcData, err := generateWebhookServices(b)
		if err != nil {
			return nil, fmt.Errorf("generating service templates: %w", err)
		}
		if len(svcData) > 0 {
			files[chartDir+"/templates/service.yaml"] = svcData
		}

		certData, err := generateCertProvider(b)
		if err != nil {
			return nil, fmt.Errorf("generating cert-manager templates: %w", err)
		}
		if len(certData) > 0 {
			files[chartDir+"/templates/cert-manager.yaml"] = certData
		}
	}

	// Additional resources
	addlData, err := generateAdditional(b)
	if err != nil {
		return nil, fmt.Errorf("generating additional templates: %w", err)
	}
	if len(addlData) > 0 {
		files[chartDir+"/templates/additional.yaml"] = addlData
	}

	return files, nil
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
func escapeHelm(s string) string {
	return strings.ReplaceAll(s, "{{", `{{ "{{" }}`)
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
	webhookServiceName := render.ObjectNameForBaseAndSuffix(strings.ReplaceAll(depName, ".", "-"), "service")
	return render.ObjectNameForBaseAndSuffix(webhookServiceName, "cert")
}

// serviceNameForDeployment returns the webhook service name for a given deployment name.
func serviceNameForDeployment(depName string) string {
	return render.ObjectNameForBaseAndSuffix(strings.ReplaceAll(depName, ".", "-"), "service")
}

// saNameOrDefault returns "default" when saName is empty.
func saNameOrDefault(saName string) string {
	return cmp.Or(saName, "default")
}
