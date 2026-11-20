package mask

import (
	"image"

	"github.com/anthonynsimon/bild/clone"
	"github.com/anthonynsimon/bild/parallel"
)

// Composite overlays the foreground image fg onto the background image bg,
// restricting the coverage of fg to the regions marked by the mask m, and
// returns the composited image.
//
// Geometry: the result has exactly the bounds of bg, preserving its global
// coordinates. All three images are aligned by global coordinates: the
// output pixel at global position (x, y) combines bg, fg and m sampled at
// the same global position (x, y), so the images may have different sizes
// and non-zero origins. A mask may therefore be local (smaller than bg):
// samples outside m.Bounds() count as 0 (no coverage), and samples outside
// fg.Bounds() count as fully transparent.
//
// Blending: inputs are converted to RGBA (premultiplied) 8-bit channels.
// Per pixel, with bA and fA in [0, 1]:
//
//	cover = clamp01(opacity) * clamp01(maskStrength) * (maskValue / 255)
//	a     = fA * cover
//	outC  = fC*a + bC*(1-a)   for R, G, B (straight channels)
//	outA  = a + bA*(1-a)
//
// Each channel is stored as uint8(value*255 + 0.5), i.e. rounded half away
// from zero and clamped to [0, 255]. opacity and maskStrength are clamped
// to [0, 1]; opacity scales the foreground as a whole, maskStrength scales
// the mask values only.
//
// The result is newly allocated and never shares its Pix slice with any of
// the inputs, which are never modified.
func Composite(bg, fg image.Image, m *image.Gray, opacity, maskStrength float64) (*image.RGBA, error) {
	if bg == nil || fg == nil {
		return nil, ErrNilImage
	}
	if m == nil {
		return nil, ErrNilMask
	}
	bounds := bg.Bounds()
	w, h, err := checkBounds(bounds)
	if err != nil {
		return nil, err
	}

	opacity = clamp01(opacity)
	maskStrength = clamp01(maskStrength)

	bgSrc := clone.AsShallowRGBA(bg)
	fgSrc := clone.AsShallowRGBA(fg)
	fgBounds := fgSrc.Bounds()
	maskBounds := m.Bounds()

	dst := image.NewRGBA(bounds)

	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			gy := bounds.Min.Y + y
			for x := 0; x < w; x++ {
				gx := bounds.Min.X + x
				p := image.Point{X: gx, Y: gy}

				bpos := bgSrc.PixOffset(gx, gy)
				br := bgSrc.Pix[bpos+0]
				bg_ := bgSrc.Pix[bpos+1]
				bb := bgSrc.Pix[bpos+2]
				ba := bgSrc.Pix[bpos+3]

				var mv uint8
				if p.In(maskBounds) {
					mv = m.Pix[m.PixOffset(gx, gy)]
				}

				cover := opacity * maskStrength * (float64(mv) / 255)

				var a float64
				var fr, fg_, fb uint8
				if cover > 0 && p.In(fgBounds) {
					fpos := fgSrc.PixOffset(gx, gy)
					fr = fgSrc.Pix[fpos+0]
					fg_ = fgSrc.Pix[fpos+1]
					fb = fgSrc.Pix[fpos+2]
					a = (float64(fgSrc.Pix[fpos+3]) / 255) * cover
				}

				dpos := y*dst.Stride + x*4
				if a <= 0 {
					dst.Pix[dpos+0] = br
					dst.Pix[dpos+1] = bg_
					dst.Pix[dpos+2] = bb
					dst.Pix[dpos+3] = ba
					continue
				}

				ia := 1 - a
				dst.Pix[dpos+0] = round8(float64(fr)*a + float64(br)*ia)
				dst.Pix[dpos+1] = round8(float64(fg_)*a + float64(bg_)*ia)
				dst.Pix[dpos+2] = round8(float64(fb)*a + float64(bb)*ia)
				dst.Pix[dpos+3] = round8(255 * (a + (float64(ba)/255)*ia))
			}
		}
	})

	return dst, nil
}

// clamp01 constrains v to the range [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// round8 rounds v half away from zero and clamps the result to [0, 255].
func round8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
