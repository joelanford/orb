package termimage

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/muesli/termenv"

	"github.com/joelanford/orb/internal/termimage/internal/oksvg"
	"github.com/joelanford/orb/internal/termimage/internal/rasterx"
)

// Render decodes an image from imgData and writes a half-block pixel
// representation to w, fitting within maxWidth terminal columns.
// It uses the Unicode lower-half block character (U+2584) with truecolor
// ANSI escape sequences to render two vertical pixels per character cell.
// If mediaType starts with "image/svg", the SVG is rasterized first.
func Render(w io.Writer, imgData []byte, mediaType string, maxWidth int) error {
	var img image.Image
	if strings.HasPrefix(mediaType, "image/svg") {
		var err error
		img, err = rasterizeSVG(imgData, maxWidth)
		if err != nil {
			return fmt.Errorf("rasterizing SVG: %w", err)
		}
	} else {
		var err error
		img, _, err = image.Decode(bytes.NewReader(imgData))
		if err != nil {
			return fmt.Errorf("decoding image: %w", err)
		}
	}
	return renderImage(w, img, maxWidth)
}

// rasterizeSVG parses SVG data in strict mode and rasterizes it to an image
// that fits within maxWidth, maintaining the original aspect ratio.
func rasterizeSVG(data []byte, maxWidth int) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}

	vbW := icon.ViewBox.W
	vbH := icon.ViewBox.H
	if vbW <= 0 || vbH <= 0 {
		return nil, fmt.Errorf("invalid SVG viewbox: %gx%g", vbW, vbH)
	}

	// SVGs are vector graphics, so always scale to maxWidth.
	dstW := float64(maxWidth)
	dstH := vbH * dstW / vbW

	w := int(dstW)
	h := int(dstH)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid rasterized dimensions: %dx%d", w, h)
	}

	icon.SetTarget(0, 0, dstW, dstH)
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	dasher := rasterx.NewDasher(w, h, scanner)

	icon.Draw(dasher, 1.0)
	return rgba, nil
}

// renderImage writes the half-block pixel representation of img to w.
func renderImage(w io.Writer, img image.Image, maxWidth int) error {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW == 0 || srcH == 0 {
		return nil
	}

	// Scale to fit maxWidth, never upscale.
	dstW := min(srcW, maxWidth)
	dstH := srcH * dstW / srcW

	if dstW == 0 || dstH == 0 {
		return nil
	}

	// Detect terminal background color, fall back to white.
	bgR, bgG, bgB := detectBackground()

	// Render using half-block technique: each character cell represents
	// two vertical pixels. The top pixel uses the background color and
	// the bottom pixel uses the foreground color of U+2584 (lower half block).
	for y := 0; y < dstH; y += 2 {
		for x := 0; x < dstW; x++ {
			// Map destination pixel to source pixel (nearest-neighbor).
			sx := bounds.Min.X + x*srcW/dstW
			sy := bounds.Min.Y + y*srcH/dstH

			topR, topG, topB := blendAlpha(img.At(sx, sy), bgR, bgG, bgB)

			// Bottom pixel: may be out of bounds for odd heights.
			var botR, botG, botB uint8
			if y+1 < dstH {
				sy2 := bounds.Min.Y + (y+1)*srcH/dstH
				botR, botG, botB = blendAlpha(img.At(sx, sy2), bgR, bgG, bgB)
			} else {
				// Odd height: bottom half matches terminal background.
				botR, botG, botB = bgR, bgG, bgB
			}

			// \033[48;2;R;G;Bm = background (top pixel)
			// \033[38;2;R;G;Bm = foreground (bottom pixel)
			if _, err := fmt.Fprintf(w, "\033[48;2;%d;%d;%dm\033[38;2;%d;%d;%dm\u2584",
				topR, topG, topB,
				botR, botG, botB,
			); err != nil {
				return err
			}
		}
		// Reset attributes and newline.
		if _, err := fmt.Fprint(w, "\033[0m\n"); err != nil {
			return err
		}
	}

	return nil
}

// detectBackground queries the terminal for its background color.
// Falls back to white if detection fails.
func detectBackground() (uint8, uint8, uint8) {
	bg := termenv.BackgroundColor()
	if _, ok := bg.(termenv.NoColor); ok {
		return 255, 255, 255
	}
	c := termenv.ConvertToRGB(bg)
	return uint8(c.R * 255), uint8(c.G * 255), uint8(c.B * 255)
}

// blendAlpha alpha-blends a color against the given background color and
// returns the resulting 8-bit RGB values.
func blendAlpha(c color.Color, bgR, bgG, bgB uint8) (uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return bgR, bgG, bgB
	}
	if a == 0xffff {
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
	af := float64(a) / 0xffff
	blend := func(v uint32, bg uint8) uint8 {
		return uint8(float64(v)/0xffff*af*255 + (1-af)*float64(bg))
	}
	return blend(r, bgR), blend(g, bgG), blend(b, bgB)
}
