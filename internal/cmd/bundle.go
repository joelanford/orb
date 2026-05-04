package cmd

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/destination"
	"github.com/joelanford/orb/internal/source"
	"github.com/joelanford/orb/internal/transport"
)

// bundleConfig represents the configuration file format for bundle rendering
type bundleConfig struct {
	WatchNamespace string `json:"watchNamespace,omitempty"`
}

type convertOptions struct {
	quiet bool
}

type plainOptions struct {
	namespace    string
	configFile   string
	certProvider string
}

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Work with operator bundles",
	}
	cmd.AddCommand(newBundleConvertCmd())
	return cmd
}

func newBundleConvertCmd() *cobra.Command {
	opts := &convertOptions{}

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert a bundle from source to destination format",
		Long: `Convert a bundle from source to destination, changing between formats.

The source argument is a skopeo-style transport:ref string (regv1 format is implicit).

Supported transports:
  docker://       Container registry
  oci:            OCI image layout directory
  oci-archive:    OCI image layout archive
  dir:            Local directory
  tar:            Tar archive (compressed or uncompressed, source only)
  chart-archive:  Helm chart archive (.tgz, helm destination only)
  stdout          Standard output (plain format only)

Use a subcommand to choose the destination format:
  plain   Plain manifests
  helm    Helm chart`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opts.quiet {
				ctx := klog.NewContext(cmd.Context(), logr.Discard())
				cmd.SetContext(ctx)
			}
		},
	}

	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress output")

	cmd.AddCommand(newConvertPlainCmd())
	cmd.AddCommand(newConvertHelmCmd())

	return cmd
}

func newConvertPlainCmd() *cobra.Command {
	plainOpts := &plainOptions{}

	cmd := &cobra.Command{
		Use:   "plain SOURCE [DESTINATION]",
		Short: "Convert a bundle to plain manifests",
		Long: `Convert a bundle to plain Kubernetes manifests.

SOURCE is a skopeo-style transport:ref string (regv1 format is implicit).
DESTINATION defaults to stdout if not specified. Supported destination transports
for plain format are dir: and stdout.

Certificate providers (--cert-provider):
  cert-manager  Use cert-manager for webhook certificates
  service-ca    Use OpenShift Service CA Operator

Examples:
  orb bundle convert plain docker://quay.io/my/bundle:v1 -n operators
  orb bundle convert plain dir:/path/to/bundle -n operators
  orb bundle convert plain tar:bundle.tar.gz -n operators
  orb bundle convert plain oci:/path/to/layout:latest dir:/tmp/output -n operators
  orb bundle convert plain docker://quay.io/my/bundle:v1 -n operators --cert-provider cert-manager
  orb bundle convert plain docker://quay.io/my/bundle:v1 -n operators -c config.yaml`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvertPlain(cmd, args, plainOpts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&plainOpts.namespace, "namespace", "n", "", "Install namespace for rendered manifests (required)")
	flags.StringVarP(&plainOpts.configFile, "config", "c", "", "Bundle configuration file (YAML with watchNamespace field)")
	flags.StringVar(&plainOpts.certProvider, "cert-provider", "", "Certificate provider: cert-manager, service-ca")

	_ = cmd.MarkFlagRequired("namespace")

	return cmd
}

func newConvertHelmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helm SOURCE DESTINATION",
		Short: "Convert a bundle to a Helm chart",
		Long: `Convert a bundle to a Helm chart.

SOURCE is a skopeo-style transport:ref string (regv1 format is implicit).
DESTINATION is a transport:ref string for the output location.

Examples:
  orb bundle convert helm docker://quay.io/my/bundle:v1 dir:/tmp/chart
  orb bundle convert helm docker://quay.io/my/bundle:v1 chart-archive:/tmp/chart.tgz`,
		Args: cobra.ExactArgs(2),
		RunE: runConvertHelm,
	}

	return cmd
}

func runConvertPlain(cmd *cobra.Command, args []string, plainOpts *plainOptions) error {
	ctx := cmd.Context()

	srcRef, err := transport.ParseRef(args[0])
	if err != nil {
		return err
	}
	if srcRef.Transport == transport.Stdout {
		return fmt.Errorf("stdout cannot be used as a source transport")
	}

	destRef := transport.Ref{Transport: transport.Stdout}
	if len(args) > 1 {
		destRef, err = transport.ParseRef(args[1])
		if err != nil {
			return err
		}
	}

	// Validate cert-provider flag
	if plainOpts.certProvider != "" && plainOpts.certProvider != "cert-manager" && plainOpts.certProvider != "service-ca" {
		return fmt.Errorf("invalid --cert-provider value %q: must be 'cert-manager' or 'service-ca'", plainOpts.certProvider)
	}

	src, err := source.New(srcRef, source.Options{})
	if err != nil {
		return err
	}

	// Build render options
	var ropts []registryv1.RenderOption

	// Process config file if provided
	if plainOpts.configFile != "" {
		bundleCfg, err := loadConfig(plainOpts.configFile)
		if err != nil {
			return fmt.Errorf("loading config file: %w", err)
		}
		if bundleCfg.WatchNamespace != "" {
			ropts = append(ropts, registryv1.WithTargetNamespaces(bundleCfg.WatchNamespace))
		}
	}

	// Add certificate provider if specified
	if plainOpts.certProvider != "" {
		certProv, err := getCertificateProvider(plainOpts.certProvider)
		if err != nil {
			return err
		}
		ropts = append(ropts, registryv1.WithCertificateProvider(certProv))
	}

	dest, err := destination.NewPlain(destRef, destination.Options{
		Namespace:   plainOpts.namespace,
		ConvertOpts: ropts,
	})
	if err != nil {
		return err
	}

	b, err := src.Read(ctx)
	if err != nil {
		return err
	}

	return dest.Write(ctx, b)
}

func runConvertHelm(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	srcRef, err := transport.ParseRef(args[0])
	if err != nil {
		return err
	}
	if srcRef.Transport == transport.Stdout {
		return fmt.Errorf("stdout cannot be used as a source transport")
	}

	destRef, err := transport.ParseRef(args[1])
	if err != nil {
		return err
	}

	src, err := source.New(srcRef, source.Options{})
	if err != nil {
		return err
	}

	dest, err := destination.NewHelm(destRef, destination.Options{})
	if err != nil {
		return err
	}

	b, err := src.Read(ctx)
	if err != nil {
		return err
	}

	return dest.Write(ctx, b)
}

func getCertificateProvider(provider string) (registryv1.CertificateProvider, error) {
	switch provider {
	case "cert-manager":
		return registryv1.CertManagerCertificateProvider{}, nil
	case "service-ca":
		return registryv1.OpenshiftServiceCACertificateProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown certificate provider: %q", provider)
	}
}

func loadConfig(path string) (*bundleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg bundleConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
