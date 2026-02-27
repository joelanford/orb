package termimage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"image/jpeg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_PNG(t *testing.T) {
	imgData := encodePNG(t, 4, 4, color.NRGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	err := Render(&buf, imgData, "image/png", 20)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "\u2584", "output should contain half-block characters")
	assert.Contains(t, out, "\033[", "output should contain ANSI escape sequences")
	assert.Contains(t, out, "\033[0m", "output should contain reset sequence")
}

func TestRender_JPEG(t *testing.T) {
	// Create a JPEG image.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBuf, img, nil))

	var buf bytes.Buffer
	err := Render(&buf, jpegBuf.Bytes(), "image/jpeg", 20)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "\u2584")
}

func TestRender_InvalidData(t *testing.T) {
	var buf bytes.Buffer
	err := Render(&buf, []byte("not an image"), "", 20)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoding image")
}

func TestRender_Scaling(t *testing.T) {
	// 40x40 image should be scaled down to 20 columns.
	imgData := encodePNG(t, 40, 40, color.NRGBA{R: 0, G: 255, B: 0, A: 255})

	var buf bytes.Buffer
	err := Render(&buf, imgData, "", 20)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// Each line should have exactly 20 half-block characters (plus ANSI escapes).
	for _, line := range lines {
		count := strings.Count(line, "\u2584")
		assert.Equal(t, 20, count, "each line should have 20 half-block chars")
	}
}

func TestRender_NoUpscale(t *testing.T) {
	// 4x4 image with maxWidth=20 should NOT upscale.
	imgData := encodePNG(t, 4, 4, color.NRGBA{R: 128, G: 128, B: 128, A: 255})

	var buf bytes.Buffer
	err := Render(&buf, imgData, "", 20)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for _, line := range lines {
		count := strings.Count(line, "\u2584")
		assert.Equal(t, 4, count, "should not upscale beyond original 4px width")
	}
}

func TestRender_OddHeight(t *testing.T) {
	// 4x3 image: last row should pair the 3rd pixel row with white.
	imgData := encodePNG(t, 4, 3, color.NRGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	err := Render(&buf, imgData, "", 20)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 2, len(lines), "3px height should produce 2 rows of half-blocks")
}

func TestRender_Transparency(t *testing.T) {
	// Fully transparent pixel should blend to white.
	imgData := encodePNG(t, 2, 2, color.NRGBA{R: 255, G: 0, B: 0, A: 0})

	var buf bytes.Buffer
	err := Render(&buf, imgData, "", 20)
	require.NoError(t, err)

	// White background = 255,255,255
	assert.Contains(t, buf.String(), "48;2;255;255;255", "transparent pixels should blend to white")
}

func TestRender_SVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<rect width="20" height="20" fill="red"/>
		<circle cx="10" cy="10" r="5" fill="blue"/>
	</svg>`)

	var buf bytes.Buffer
	err := Render(&buf, svg, "image/svg+xml", 20)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "\u2584", "SVG output should contain half-block characters")
	assert.Contains(t, out, "\033[", "SVG output should contain ANSI escape sequences")
}

func TestRender_SVG_UnsupportedElement(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<foreignObject width="20" height="20"/>
	</svg>`)

	var buf bytes.Buffer
	err := Render(&buf, svg, "image/svg+xml", 20)
	assert.Error(t, err, "strict mode should reject unsupported SVG elements")
}

func TestRasterizeSVG_MatchesPNG(t *testing.T) {
	svgData, err := os.ReadFile("testdata/cockroachdb-logo-png-transparent.svg")
	require.NoError(t, err)

	pngData, err := os.ReadFile("testdata/cockroachdb-logo-png-transparent.png")
	require.NoError(t, err)

	pngImg, err := png.Decode(bytes.NewReader(pngData))
	require.NoError(t, err)

	// Rasterize SVG at the same width as the reference PNG.
	svgImg, err := rasterizeSVG(svgData, pngImg.Bounds().Dx())
	require.NoError(t, err)

	pngDist := colorDistribution(pngImg)
	svgDist := colorDistribution(svgImg)

	t.Logf("PNG  distribution: transparent=%.2f%% blue=%.2f%% green=%.2f%%",
		pngDist.transparent*100, pngDist.blue*100, pngDist.green*100)
	t.Logf("SVG  distribution: transparent=%.2f%% blue=%.2f%% green=%.2f%%",
		svgDist.transparent*100, svgDist.blue*100, svgDist.green*100)

	const tolerance = 0.05 // 5 percentage points
	assert.InDelta(t, pngDist.transparent, svgDist.transparent, tolerance, "%%transparent should be similar")
	assert.InDelta(t, pngDist.blue, svgDist.blue, tolerance, "%%blue should be similar")
	assert.InDelta(t, pngDist.green, svgDist.green, tolerance, "%%green should be similar")
}

type distribution struct {
	transparent float64
	blue        float64
	green       float64
}

// colorDistribution classifies every pixel in img as transparent, blue, or
// green and returns each category's fraction of total pixels.
func colorDistribution(img image.Image) distribution {
	bounds := img.Bounds()
	var nTransparent, nBlue, nGreen int
	total := bounds.Dx() * bounds.Dy()
	if total == 0 {
		return distribution{}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			switch {
			case a < 0x8000:
				nTransparent++
			case g > r && g > b:
				nGreen++
			default:
				nBlue++
			}
		}
	}
	n := float64(total)
	return distribution{
		transparent: float64(nTransparent) / n,
		blue:        float64(nBlue) / n,
		green:       float64(nGreen) / n,
	}
}

func encodePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
