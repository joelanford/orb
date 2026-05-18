package termimage

import (
	"encoding/base64"
	"fmt"
	"image"
	"io"
)

const (
	kittyChunkSize = 4096
	kittyMaxPixels = 512
)

// renderKitty writes img to w using the kitty terminal graphics protocol.
// The image pixel data is capped at kittyMaxPixels per side (no upscale).
// The image is displayed at its native pixel resolution — no cell-based
// scaling — so it stays sharp.
func renderKitty(w io.Writer, img image.Image) error {
	img = kittyScaleImage(img)
	rgba := extractRGBA(img)
	bounds := img.Bounds()
	pixW := bounds.Dx()
	pixH := bounds.Dy()

	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(rgba)

	for i := 0; i < len(encoded); i += kittyChunkSize {
		end := min(i+kittyChunkSize, len(encoded))
		chunk := encoded[i:end]
		isFirst := i == 0
		isLast := end == len(encoded)

		var err error
		switch {
		case isFirst && isLast:
			_, err = fmt.Fprintf(w, "\033_Ga=T,f=32,s=%d,v=%d,m=0;%s\033\\", pixW, pixH, chunk)
		case isFirst:
			_, err = fmt.Fprintf(w, "\033_Ga=T,f=32,s=%d,v=%d,m=1;%s\033\\", pixW, pixH, chunk)
		case isLast:
			_, err = fmt.Fprintf(w, "\033_Gm=0;%s\033\\", chunk)
		default:
			_, err = fmt.Fprintf(w, "\033_Gm=1;%s\033\\", chunk)
		}
		if err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(w, "\n")
	return err
}

// extractRGBA returns the raw RGBA pixel data from img in row-major order
// (R, G, B, A per pixel, left-to-right, top-to-bottom).
func extractRGBA(img image.Image) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	data := make([]byte, width*height*4)

	for y := range height {
		for x := range width {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			off := (y*width + x) * 4
			data[off] = uint8(r >> 8)
			data[off+1] = uint8(g >> 8)
			data[off+2] = uint8(b >> 8)
			data[off+3] = uint8(a >> 8)
		}
	}
	return data
}

// kittyScaleImage scales img down so that neither dimension exceeds
// kittyMaxPixels, maintaining aspect ratio. Never upscales.
func kittyScaleImage(img image.Image) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW <= kittyMaxPixels && srcH <= kittyMaxPixels {
		return img
	}

	scale := float64(kittyMaxPixels) / float64(max(srcW, srcH))
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)

	if dstW <= 0 || dstH <= 0 {
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
