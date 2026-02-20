package cmd

import (
	"github.com/spf13/cobra"

	"github.com/joelanford/orb/internal/destination"
	"github.com/joelanford/orb/internal/ref"
	"github.com/joelanford/orb/internal/source"
)

type copyOptions struct {
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

func newCopyCmd() *cobra.Command {
	opts := &copyOptions{}

	cmd := &cobra.Command{
		Use:   "copy SOURCE DESTINATION",
		Short: "Copy a bundle from source to destination",
		Long: `Copy a bundle from source to destination, converting between formats.

Arguments use the form format:transport:ref. The first colon separates the
format prefix; the remainder is a skopeo-style transport:ref string.

Supported formats:
  regv1   Registry+v1 bundle (source only)
  helm    Helm chart (destination only)
  plain   Plain manifests (destination only)

Supported transports:
  docker://  Container registry
  oci:       OCI image layout directory
  oci-archive:  OCI image layout archive
  dir:       Local directory
  stdout     Standard output (plain format only)

Examples:
  orb copy regv1:docker://quay.io/my/bundle:v1 helm:dir:/tmp/chart
  orb copy regv1:oci:/path/to/layout:latest helm:oci-archive:/path/to/chart.tar
  orb copy regv1:dir:/path/to/bundle plain:stdout`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCopy(cmd, args, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.srcUsername, "src-username", "", "Source registry username")
	flags.StringVar(&opts.srcPassword, "src-password", "", "Source registry password")
	flags.BoolVar(&opts.srcTLSVerify, "src-tls-verify", true, "Require HTTPS and verify certificates for source")
	flags.StringVar(&opts.srcCertDir, "src-cert-dir", "", "Source certificate directory")
	flags.BoolVar(&opts.srcNoCreds, "src-no-creds", false, "Do not use credentials for source")

	flags.StringVar(&opts.destUsername, "dest-username", "", "Destination registry username")
	flags.StringVar(&opts.destPassword, "dest-password", "", "Destination registry password")
	flags.BoolVar(&opts.destTLSVerify, "dest-tls-verify", true, "Require HTTPS and verify certificates for destination")
	flags.StringVar(&opts.destCertDir, "dest-cert-dir", "", "Destination certificate directory")
	flags.BoolVar(&opts.destNoCreds, "dest-no-creds", false, "Do not use credentials for destination")

	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress output")

	return cmd
}

func runCopy(cmd *cobra.Command, args []string, opts *copyOptions) error {
	srcRef, err := ref.Parse(args[0])
	if err != nil {
		return err
	}
	if err := ref.ValidateSource(srcRef); err != nil {
		return err
	}

	destRef, err := ref.Parse(args[1])
	if err != nil {
		return err
	}
	if err := ref.ValidateDestination(destRef); err != nil {
		return err
	}

	src, err := source.New(srcRef, source.Options{
		Username:  opts.srcUsername,
		Password:  opts.srcPassword,
		TLSVerify: opts.srcTLSVerify,
		CertDir:   opts.srcCertDir,
		NoCreds:   opts.srcNoCreds,
	})
	if err != nil {
		return err
	}

	dest, err := destination.New(destRef, destination.Options{
		Username:  opts.destUsername,
		Password:  opts.destPassword,
		TLSVerify: opts.destTLSVerify,
		CertDir:   opts.destCertDir,
		NoCreds:   opts.destNoCreds,
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
