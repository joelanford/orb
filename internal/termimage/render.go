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
	backend := detectBackend(w)
	if backend == backendNone {
		return nil
	}

	isSVG := strings.HasPrefix(mediaType, "image/svg")

	var img image.Image
	if isSVG {
		rasterWidth := maxWidth
		if backend == backendKitty {
			rasterWidth = kittyMaxPixels
		}
		var err error
		img, err = rasterizeSVG(imgData, rasterWidth)
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

	switch backend {
	case backendKitty:
		return renderKitty(w, img)
	default:
		img = scaleImage(img, maxWidth)
		if img == nil {
			return nil
		}
		return renderHalfBlock(w, img)
	}
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

// scaleImage scales img to fit within maxWidth using nearest-neighbor
// sampling, never upscaling. Returns nil if the result would be empty.
func scaleImage(img image.Image, maxWidth int) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW == 0 || srcH == 0 {
		return nil
	}

	dstW := min(srcW, maxWidth)
	dstH := srcH * dstW / srcW

	if dstW == 0 || dstH == 0 {
		return nil
	}

	if dstW == srcW && dstH == srcH {
		return img
	}

	scaled := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := range dstH {
		for x := range dstW {
			sx := bounds.Min.X + x*srcW/dstW
			sy := bounds.Min.Y + y*srcH/dstH
			scaled.Set(x, y, img.At(sx, sy))
		}
	}
	return scaled
}

// renderHalfBlock writes the half-block pixel representation of img to w.
// The image must already be scaled to the desired dimensions.
func renderHalfBlock(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	dstW := bounds.Dx()
	dstH := bounds.Dy()

	bgR, bgG, bgB := detectBackground()

	for y := 0; y < dstH; y += 2 {
		for x := 0; x < dstW; x++ {
			topR, topG, topB := blendAlpha(img.At(bounds.Min.X+x, bounds.Min.Y+y), bgR, bgG, bgB)

			var botR, botG, botB uint8
			if y+1 < dstH {
				botR, botG, botB = blendAlpha(img.At(bounds.Min.X+x, bounds.Min.Y+y+1), bgR, bgG, bgB)
			} else {
				botR, botG, botB = bgR, bgG, bgB
			}

			if _, err := fmt.Fprintf(w, "\033[48;2;%d;%d;%dm\033[38;2;%d;%d;%dm\u2584",
				topR, topG, topB,
				botR, botG, botB,
			); err != nil {
				return err
			}
		}
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
