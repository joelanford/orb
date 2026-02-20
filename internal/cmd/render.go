package cmd

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/destination"
	"github.com/joelanford/orb/internal/render"
	"github.com/joelanford/orb/internal/render/certproviders"
	"github.com/joelanford/orb/internal/source"
	"github.com/joelanford/orb/internal/transport"
)

// bundleConfig represents the configuration file format for bundle rendering
type bundleConfig struct {
	WatchNamespace string `json:"watchNamespace,omitempty"`
}

type renderOptions struct {
	srcUsername  string
	srcPassword string
	srcTLSVerify bool
	srcCertDir  string
	srcNoCreds  bool

	destUsername  string
	destPassword string
	destTLSVerify bool
	destCertDir  string
	destNoCreds  bool

	quiet bool
}

type plainOptions struct {
	namespace    string
	configFile   string
	certProvider string
}

func newRenderCmd() *cobra.Command {
	opts := &renderOptions{}

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render a bundle from source to destination",
		Long: `Render a bundle from source to destination, converting between formats.

The source argument is a skopeo-style transport:ref string (regv1 format is implicit).

Supported transports:
  docker://     Container registry
  oci:          OCI image layout directory
  oci-archive:  OCI image layout archive
  dir:          Local directory
  tar:          Tar archive (compressed or uncompressed, source only)
  stdout        Standard output (plain format only)

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

	pflags := cmd.PersistentFlags()
	pflags.StringVar(&opts.srcUsername, "src-username", "", "Source registry username")
	pflags.StringVar(&opts.srcPassword, "src-password", "", "Source registry password")
	pflags.BoolVar(&opts.srcTLSVerify, "src-tls-verify", true, "Require HTTPS and verify certificates for source")
	pflags.StringVar(&opts.srcCertDir, "src-cert-dir", "", "Source certificate directory")
	pflags.BoolVar(&opts.srcNoCreds, "src-no-creds", false, "Do not use credentials for source")

	pflags.StringVar(&opts.destUsername, "dest-username", "", "Destination registry username")
	pflags.StringVar(&opts.destPassword, "dest-password", "", "Destination registry password")
	pflags.BoolVar(&opts.destTLSVerify, "dest-tls-verify", true, "Require HTTPS and verify certificates for destination")
	pflags.StringVar(&opts.destCertDir, "dest-cert-dir", "", "Destination certificate directory")
	pflags.BoolVar(&opts.destNoCreds, "dest-no-creds", false, "Do not use credentials for destination")

	pflags.BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress output")

	cmd.AddCommand(newRenderPlainCmd(opts))
	cmd.AddCommand(newRenderHelmCmd(opts))

	return cmd
}

func newRenderPlainCmd(renderOpts *renderOptions) *cobra.Command {
	plainOpts := &plainOptions{}

	cmd := &cobra.Command{
		Use:   "plain SOURCE [DESTINATION]",
		Short: "Render a bundle as plain manifests",
		Long: `Render a bundle as plain Kubernetes manifests.

SOURCE is a skopeo-style transport:ref string (regv1 format is implicit).
DESTINATION defaults to stdout if not specified. Supported destination transports
for plain format are dir: and stdout.

Certificate providers (--cert-provider):
  cert-manager  Use cert-manager for webhook certificates
  service-ca    Use OpenShift Service CA Operator

Examples:
  orb render plain docker://quay.io/my/bundle:v1 -n operators
  orb render plain dir:/path/to/bundle -n operators
  orb render plain tar:bundle.tar.gz -n operators
  orb render plain oci:/path/to/layout:latest dir:/tmp/output -n operators
  orb render plain docker://quay.io/my/bundle:v1 -n operators --cert-provider cert-manager
  orb render plain docker://quay.io/my/bundle:v1 -n operators -c config.yaml`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRenderPlain(cmd, args, renderOpts, plainOpts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&plainOpts.namespace, "namespace", "n", "", "Install namespace for rendered manifests (required)")
	flags.StringVarP(&plainOpts.configFile, "config", "c", "", "Bundle configuration file (YAML with watchNamespace field)")
	flags.StringVar(&plainOpts.certProvider, "cert-provider", "", "Certificate provider: cert-manager, service-ca")

	_ = cmd.MarkFlagRequired("namespace")

	return cmd
}

func newRenderHelmCmd(renderOpts *renderOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helm SOURCE DESTINATION",
		Short: "Render a bundle as a Helm chart",
		Long: `Render a bundle as a Helm chart.

SOURCE is a skopeo-style transport:ref string (regv1 format is implicit).
DESTINATION is a transport:ref string for the output location.

Examples:
  orb render helm docker://quay.io/my/bundle:v1 dir:/tmp/chart
  orb render helm docker://quay.io/my/bundle:v1 docker://quay.io/my/chart:v1
  orb render helm oci:/path/to/layout:latest oci-archive:/tmp/chart.tar`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRenderHelm(cmd, args, renderOpts)
		},
	}

	return cmd
}

func runRenderPlain(cmd *cobra.Command, args []string, renderOpts *renderOptions, plainOpts *plainOptions) error {
	srcRef, err := transport.ParseTransportRef(args[0])
	if err != nil {
		return err
	}
	if srcRef.Transport == transport.Stdout {
		return fmt.Errorf("stdout cannot be used as a source transport")
	}

	destRef := transport.TransportRef{Transport: transport.Stdout}
	if len(args) > 1 {
		destRef, err = transport.ParseTransportRef(args[1])
		if err != nil {
			return err
		}
	}

	// Validate cert-provider flag
	if plainOpts.certProvider != "" && plainOpts.certProvider != "cert-manager" && plainOpts.certProvider != "service-ca" {
		return fmt.Errorf("invalid --cert-provider value %q: must be 'cert-manager' or 'service-ca'", plainOpts.certProvider)
	}

	src, err := source.New(srcRef, source.Options{
		Username:  renderOpts.srcUsername,
		Password:  renderOpts.srcPassword,
		TLSVerify: renderOpts.srcTLSVerify,
		CertDir:   renderOpts.srcCertDir,
		NoCreds:   renderOpts.srcNoCreds,
	})
	if err != nil {
		return err
	}

	// Build render options
	var ropts []render.Option

	// Process config file if provided
	if plainOpts.configFile != "" {
		bundleCfg, err := loadConfig(plainOpts.configFile)
		if err != nil {
			return fmt.Errorf("loading config file: %w", err)
		}
		if bundleCfg.WatchNamespace != "" {
			ropts = append(ropts, render.WithTargetNamespaces(bundleCfg.WatchNamespace))
		}
	}

	// Add certificate provider if specified
	if plainOpts.certProvider != "" {
		certProv, err := getCertificateProvider(plainOpts.certProvider)
		if err != nil {
			return err
		}
		ropts = append(ropts, render.WithCertificateProvider(certProv))
	}

	dest, err := destination.NewPlain(destRef, destination.Options{
		Username:   renderOpts.destUsername,
		Password:   renderOpts.destPassword,
		TLSVerify:  renderOpts.destTLSVerify,
		CertDir:    renderOpts.destCertDir,
		NoCreds:    renderOpts.destNoCreds,
		Namespace:  plainOpts.namespace,
		RenderOpts: ropts,
	})
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	b, err := src.Read(ctx)
	if err != nil {
		return err
	}

	return dest.Write(ctx, b)
}

func runRenderHelm(cmd *cobra.Command, args []string, renderOpts *renderOptions) error {
	srcRef, err := transport.ParseTransportRef(args[0])
	if err != nil {
		return err
	}
	if srcRef.Transport == transport.Stdout {
		return fmt.Errorf("stdout cannot be used as a source transport")
	}

	destRef, err := transport.ParseTransportRef(args[1])
	if err != nil {
		return err
	}

	src, err := source.New(srcRef, source.Options{
		Username:  renderOpts.srcUsername,
		Password:  renderOpts.srcPassword,
		TLSVerify: renderOpts.srcTLSVerify,
		CertDir:   renderOpts.srcCertDir,
		NoCreds:   renderOpts.srcNoCreds,
	})
	if err != nil {
		return err
	}

	dest, err := destination.NewHelm(destRef, destination.Options{
		Username:  renderOpts.destUsername,
		Password:  renderOpts.destPassword,
		TLSVerify: renderOpts.destTLSVerify,
		CertDir:   renderOpts.destCertDir,
		NoCreds:   renderOpts.destNoCreds,
	})
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	b, err := src.Read(ctx)
	if err != nil {
		return err
	}

	return dest.Write(ctx, b)
}

func getCertificateProvider(provider string) (render.CertificateProvider, error) {
	switch provider {
	case "cert-manager":
		return certproviders.CertManagerCertificateProvider{}, nil
	case "service-ca":
		return certproviders.OpenshiftServiceCaCertificateProvider{}, nil
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
