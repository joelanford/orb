package source

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"go.podman.io/image/v5/pkg/compression"
	"go.podman.io/image/v5/types"

	dockerTransport "go.podman.io/image/v5/docker"
	archiveTransport "go.podman.io/image/v5/oci/archive"
	layoutTransport "go.podman.io/image/v5/oci/layout"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/joelanford/orb/internal/image"
)

type regv1Docker struct {
	ref  string
	opts Options
}

func (s *regv1Docker) Read(ctx context.Context) (*bundle.RegistryV1, error) {
	imgRef, err := dockerTransport.ParseReference("//" + s.ref)
	if err != nil {
		return nil, fmt.Errorf("parsing docker reference: %w", err)
	}
	return readFromImage(ctx, imgRef, buildSystemContext(s.opts))
}

type regv1OCI struct {
	ref  string
	opts Options
}

func (s *regv1OCI) Read(ctx context.Context) (*bundle.RegistryV1, error) {
	imgRef, err := layoutTransport.ParseReference(s.ref)
	if err != nil {
		return nil, fmt.Errorf("parsing oci layout reference: %w", err)
	}
	return readFromImage(ctx, imgRef, buildSystemContext(s.opts))
}

type regv1OCIArchive struct {
	ref  string
	opts Options
}

func (s *regv1OCIArchive) Read(ctx context.Context) (*bundle.RegistryV1, error) {
	imgRef, err := archiveTransport.ParseReference(s.ref)
	if err != nil {
		return nil, fmt.Errorf("parsing oci-archive reference: %w", err)
	}
	return readFromImage(ctx, imgRef, buildSystemContext(s.opts))
}

type regv1Dir struct {
	ref  string
	opts Options
}

func (s *regv1Dir) Read(_ context.Context) (*bundle.RegistryV1, error) {
	rv1, err := bundle.FromFS(os.DirFS(expandPath(s.ref)))
	if err != nil {
		return nil, err
	}
	return &rv1, nil
}

type regv1Tar struct {
	ref  string
	opts Options
}

func (s *regv1Tar) Read(_ context.Context) (*bundle.RegistryV1, error) {
	tmpDir, err := os.MkdirTemp("", "orb-tar-")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	f, err := os.Open(expandPath(s.ref))
	if err != nil {
		return nil, fmt.Errorf("opening tar file: %w", err)
	}
	defer f.Close()

	// Auto-decompress (handles gzip, bzip2, xz, zstd, or plain tar)
	decompressed, _, err := compression.AutoDecompress(f)
	if err != nil {
		return nil, fmt.Errorf("decompressing archive: %w", err)
	}
	defer decompressed.Close()

	if err := untar(decompressed, tmpDir); err != nil {
		return nil, fmt.Errorf("extracting tar: %w", err)
	}

	rv1, err := bundle.FromFS(os.DirFS(tmpDir))
	if err != nil {
		return nil, fmt.Errorf("parsing bundle from tar: %w", err)
	}
	return &rv1, nil
}

func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		// Clean the path and skip entries that resolve to the root
		cleanName := filepath.Clean(header.Name)
		if cleanName == "." || cleanName == "/" {
			continue
		}

		// Sanitize path to prevent directory traversal
		target := filepath.Join(dest, cleanName)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			// Limit copy to prevent decompression bombs
			if _, err := io.Copy(outFile, io.LimitReader(tr, 100*1024*1024)); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// readFromImage extracts a registry+v1 bundle from an OCI image using the imagev2 stack.
func readFromImage(ctx context.Context, imgRef types.ImageReference, sysCtx *types.SystemContext) (*bundle.RegistryV1, error) {
	client, err := image.NewContainersImageClient(ctx, imgRef, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("creating image client: %w", err)
	}

	repo, err := image.NewCachingRepository(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("creating caching repository: %w", err)
	}
	defer repo.Close()

	tmpDir, err := os.MkdirTemp("", "orb-bundle-")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	resolver := image.NewResolver()
	resolver.Register(&image.RegistryV1Handler{})

	if err := resolver.Unpack(ctx, repo, tmpDir); err != nil {
		return nil, fmt.Errorf("unpacking image: %w", err)
	}

	rv1, err := bundle.FromFS(os.DirFS(tmpDir))
	if err != nil {
		return nil, fmt.Errorf("parsing bundle: %w", err)
	}
	return &rv1, nil
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if u, err := user.Current(); err == nil {
			return filepath.Join(u.HomeDir, path[2:])
		}
	}
	return path
}

// buildSystemContext maps source Options to a podman SystemContext.
func buildSystemContext(opts Options) *types.SystemContext {
	sysCtx := &types.SystemContext{}

	if !opts.TLSVerify {
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
		sysCtx.OCIInsecureSkipTLSVerify = true
	}

	if opts.CertDir != "" {
		sysCtx.DockerCertPath = opts.CertDir
	}

	if !opts.NoCreds && opts.Username != "" {
		sysCtx.DockerAuthConfig = &types.DockerAuthConfig{
			Username: opts.Username,
			Password: opts.Password,
		}
	}

	return sysCtx
}
