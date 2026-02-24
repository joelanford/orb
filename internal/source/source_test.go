package source

import (
	"archive/tar"
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"

	"github.com/joelanford/orb/internal/transport"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		transport transport.Transport
		wantType  string
		wantErr   bool
	}{
		{"Docker", transport.Docker, "*source.regv1Docker", false},
		{"OCI", transport.OCI, "*source.regv1OCI", false},
		{"OCIArchive", transport.OCIArchive, "*source.regv1OCIArchive", false},
		{"Dir", transport.Dir, "*source.regv1Dir", false},
		{"Tar", transport.Tar, "*source.regv1Tar", false},
		{"Unsupported", transport.Stdout, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := New(transport.Ref{Transport: tt.transport, Ref: "test-ref"}, Options{})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported")
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, src)
		})
	}
}

func TestExpandPath(t *testing.T) {
	t.Run("AbsolutePath", func(t *testing.T) {
		result := transport.ExpandPath("/some/absolute/path")
		assert.Equal(t, "/some/absolute/path", result)
	})

	t.Run("TildePath", func(t *testing.T) {
		u, err := user.Current()
		require.NoError(t, err)
		result := transport.ExpandPath("~/somedir")
		assert.Equal(t, filepath.Join(u.HomeDir, "somedir"), result)
	})

	t.Run("RelativePath", func(t *testing.T) {
		result := transport.ExpandPath("relative/path")
		assert.Equal(t, "relative/path", result)
	})
}

func TestBuildSystemContext(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		// When TLSVerify is false (the zero value), insecure flags are set
		ctx := buildSystemContext(Options{})
		assert.Equal(t, types.OptionalBoolTrue, ctx.DockerInsecureSkipTLSVerify)
		assert.True(t, ctx.OCIInsecureSkipTLSVerify)
		assert.Nil(t, ctx.DockerAuthConfig)
		assert.Empty(t, ctx.DockerCertPath)
	})

	t.Run("TLSVerifyTrue", func(t *testing.T) {
		ctx := buildSystemContext(Options{TLSVerify: true})
		assert.Equal(t, types.OptionalBoolUndefined, ctx.DockerInsecureSkipTLSVerify)
		assert.False(t, ctx.OCIInsecureSkipTLSVerify)
	})

	t.Run("WithCredentials", func(t *testing.T) {
		ctx := buildSystemContext(Options{Username: "user", Password: "pass"})
		require.NotNil(t, ctx.DockerAuthConfig)
		assert.Equal(t, "user", ctx.DockerAuthConfig.Username)
		assert.Equal(t, "pass", ctx.DockerAuthConfig.Password)
	})

	t.Run("NoCreds", func(t *testing.T) {
		ctx := buildSystemContext(Options{Username: "user", Password: "pass", NoCreds: true})
		assert.Nil(t, ctx.DockerAuthConfig)
	})

	t.Run("WithCertDir", func(t *testing.T) {
		ctx := buildSystemContext(Options{CertDir: "/certs"})
		assert.Equal(t, "/certs", ctx.DockerCertPath)
	})
}

func TestUntar(t *testing.T) {
	t.Run("RegularFiles", func(t *testing.T) {
		dest := t.TempDir()
		buf := createTar(t, []tarEntry{
			{Name: "file.txt", Typeflag: tar.TypeReg, Content: []byte("hello")},
		})
		require.NoError(t, untar(buf, dest))

		data, err := os.ReadFile(filepath.Join(dest, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(data))
	})

	t.Run("Directories", func(t *testing.T) {
		dest := t.TempDir()
		buf := createTar(t, []tarEntry{
			{Name: "subdir/", Typeflag: tar.TypeDir},
			{Name: "subdir/file.txt", Typeflag: tar.TypeReg, Content: []byte("data")},
		})
		require.NoError(t, untar(buf, dest))

		info, err := os.Stat(filepath.Join(dest, "subdir"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		data, err := os.ReadFile(filepath.Join(dest, "subdir", "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "data", string(data))
	})

	t.Run("Symlinks", func(t *testing.T) {
		dest := t.TempDir()
		buf := createTar(t, []tarEntry{
			{Name: "target.txt", Typeflag: tar.TypeReg, Content: []byte("target")},
			{Name: "link.txt", Typeflag: tar.TypeSymlink, Linkname: "target.txt"},
		})
		require.NoError(t, untar(buf, dest))

		linkTarget, err := os.Readlink(filepath.Join(dest, "link.txt"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", linkTarget)
	})

	t.Run("PathTraversalRejected", func(t *testing.T) {
		dest := t.TempDir()
		buf := createTar(t, []tarEntry{
			{Name: "../../../etc/passwd", Typeflag: tar.TypeReg, Content: []byte("evil")},
		})
		err := untar(buf, dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tar path")
	})

	t.Run("RootEntriesSkipped", func(t *testing.T) {
		dest := t.TempDir()
		buf := createTar(t, []tarEntry{
			{Name: "./", Typeflag: tar.TypeDir},
			{Name: "file.txt", Typeflag: tar.TypeReg, Content: []byte("data")},
		})
		require.NoError(t, untar(buf, dest))

		data, err := os.ReadFile(filepath.Join(dest, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "data", string(data))
	})
}

type tarEntry struct {
	Name     string
	Typeflag byte
	Content  []byte
	Linkname string
}

func createTar(t *testing.T, entries []tarEntry) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Typeflag: e.Typeflag,
			Size:     int64(len(e.Content)),
			Mode:     0644,
		}
		if e.Typeflag == tar.TypeDir {
			hdr.Mode = 0755
		}
		if e.Typeflag == tar.TypeSymlink {
			hdr.Linkname = e.Linkname
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if len(e.Content) > 0 {
			_, err := tw.Write(e.Content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return bytes.NewReader(buf.Bytes())
}
