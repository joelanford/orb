package termimage

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRGBA_SolidColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			img.Set(x, y, color.NRGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}

	data := extractRGBA(img)
	assert.Len(t, data, 2*2*4)

	for i := 0; i < len(data); i += 4 {
		assert.Equal(t, uint8(255), data[i], "R at offset %d", i)
		assert.Equal(t, uint8(128), data[i+1], "G at offset %d", i)
		assert.Equal(t, uint8(0), data[i+2], "B at offset %d", i)
		assert.Equal(t, uint8(255), data[i+3], "A at offset %d", i)
	}
}

func TestExtractRGBA_Transparent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})

	data := extractRGBA(img)
	assert.Equal(t, []byte{0, 0, 0, 0}, data)
}

func TestExtractRGBA_NonNRGBA(t *testing.T) {
	// RGBA image (premultiplied) — extractRGBA should still work.
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 100, G: 200, B: 50, A: 255})

	data := extractRGBA(img)
	assert.Len(t, data, 4)
	assert.Equal(t, uint8(100), data[0])
	assert.Equal(t, uint8(200), data[1])
	assert.Equal(t, uint8(50), data[2])
	assert.Equal(t, uint8(255), data[3])
}

func TestRenderKitty_SmallImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			img.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	var buf bytes.Buffer
	err := renderKitty(&buf, img)
	require.NoError(t, err)

	out := buf.String()

	// Should start with the kitty APC sequence.
	assert.Contains(t, out, "\033_G")
	// Should contain the correct format and pixel dimensions.
	assert.Contains(t, out, "a=T,f=32,s=2,v=2")
	// Small image should fit in a single chunk (m=0).
	assert.Contains(t, out, "m=0;")
	// Should end with ST and a newline.
	assert.True(t, strings.HasSuffix(out, "\033\\\n"))

	// Verify the base64 payload decodes to correct RGBA data.
	parts := strings.SplitN(out, ";", 2)
	require.Len(t, parts, 2)
	b64Data := strings.TrimSuffix(parts[1], "\033\\\n")
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	require.NoError(t, err)
	assert.Len(t, decoded, 2*2*4)
	// Each pixel: 255, 0, 0, 255
	for i := 0; i < len(decoded); i += 4 {
		assert.Equal(t, uint8(255), decoded[i])
		assert.Equal(t, uint8(0), decoded[i+1])
		assert.Equal(t, uint8(0), decoded[i+2])
		assert.Equal(t, uint8(255), decoded[i+3])
	}
}

func TestRenderKitty_Chunking(t *testing.T) {
	// Create an image large enough to require multiple chunks.
	// Each pixel is 4 bytes, base64 encoding expands by 4/3.
	// kittyChunkSize is 4096 bytes of base64.
	// 4096 bytes base64 = 3072 bytes raw = 768 pixels.
	// A 30x30 image = 900 pixels = 3600 bytes raw = 4800 bytes base64 > 4096.
	img := image.NewNRGBA(image.Rect(0, 0, 30, 30))
	for y := range 30 {
		for x := range 30 {
			img.Set(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	err := renderKitty(&buf, img)
	require.NoError(t, err)

	out := buf.String()

	// Count the number of escape sequences (\033_G).
	chunks := strings.Count(out, "\033_G")
	assert.GreaterOrEqual(t, chunks, 2, "should have multiple chunks for a 30x30 image")

	// First chunk should have m=1 (more data).
	assert.Contains(t, out, fmt.Sprintf("a=T,f=32,s=%d,v=%d,m=1;", 30, 30))
	// Last chunk should have m=0.
	lastChunkIdx := strings.LastIndex(out, "\033_G")
	lastChunk := out[lastChunkIdx:]
	assert.Contains(t, lastChunk, "m=0;")
}

func TestRenderKitty_1x1(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 42, G: 84, B: 126, A: 200})

	var buf bytes.Buffer
	err := renderKitty(&buf, img)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "s=1,v=1")
	assert.Contains(t, out, "m=0;")
}
