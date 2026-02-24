package destination

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/convert"
	"github.com/joelanford/orb/internal/transport"
)

type plainDir struct {
	ref  string
	opts Options
}

func (d *plainDir) Write(_ context.Context, b *bundle.RegistryV1) error {
	dir := transport.ExpandPath(d.ref)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("destination directory already exists: %s", dir)
	}

	objs, err := convert.Converter.Convert(*b, d.opts.Namespace, d.opts.ConvertOpts...)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	outPath := filepath.Join(dir, "manifests.yaml")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	return outputYAML(f, objs)
}

type plainStdout struct {
	opts Options
}

func (d *plainStdout) Write(_ context.Context, b *bundle.RegistryV1) error {
	objs, err := convert.Converter.Convert(*b, d.opts.Namespace, d.opts.ConvertOpts...)
	if err != nil {
		return err
	}
	return outputYAML(os.Stdout, objs)
}

func outputYAML(w io.Writer, objs []client.Object) error {
	for i, obj := range objs {
		if i > 0 {
			if _, err := fmt.Fprintln(w, "---"); err != nil {
				return err
			}
		}

		data, err := yaml.Marshal(obj)
		if err != nil {
			return fmt.Errorf("marshaling object %s/%s: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
		}

		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}
