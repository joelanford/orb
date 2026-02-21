package convert

// Converter converts registry+v1 bundles into plain kubernetes manifests
var Converter = BundleConverter{
	BundleValidator:    bundleValidator,
	ResourceGenerators: resourceGenerators,
}

var bundleValidator = BundleValidator{
	CheckDeploymentSpecUniqueness,
	CheckDeploymentNameIsDNS1123SubDomain,
	CheckCRDResourceUniqueness,
	CheckOwnedCRDExistence,
	CheckPackageNameNotEmpty,
	CheckConversionWebhookSupport,
	CheckWebhookDeploymentReferentialIntegrity,
	CheckWebhookNameUniqueness,
	CheckWebhookNameIsDNS1123SubDomain,
	CheckConversionWebhookCRDReferenceUniqueness,
	CheckConversionWebhooksReferenceOwnedCRDs,
	CheckWebhookRules,
}

var resourceGenerators = []ResourceGenerator{
	BundleCSVServiceAccountGenerator,
	BundleCSVPermissionsGenerator,
	BundleCSVClusterPermissionsGenerator,
	BundleCRDGenerator,
	BundleAdditionalResourcesGenerator,
	BundleCSVDeploymentGenerator,
	BundleValidatingWebhookResourceGenerator,
	BundleMutatingWebhookResourceGenerator,
	BundleDeploymentServiceResourceGenerator,
	CertProviderResourceGenerator,
}
