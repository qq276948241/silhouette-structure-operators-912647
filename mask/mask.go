/*
Package mask provides binary mask creation from arbitrary images,
morphological operations (dilate, erode, open, close) over structuring
elements, and mask-guided compositing of two images.

All functions preserve the global coordinates of their inputs: output
images share the bounds (including a non-zero origin) of the governing
input, and inputs are never modified.
*/
package mask

import (
	"errors"
	"image"
	"image/color"

	"github.com/anthonynsimon/bild/parallel"
	"github.com/anthonynsimon/bild/util"
)

// MaxPixels is the pixel budget for any single mask operation. Requests
// whose bounds contain more pixels are rejected with ErrTooLarge before
// any work buffer is allocated.
const MaxPixels = 1 << 28

var (
	// ErrNilImage is returned when a required image is nil.
	ErrNilImage = errors.New("mask: image must not be nil")
	// ErrNilMask is returned when a required mask is nil.
	ErrNilMask = errors.New("mask: mask must not be nil")
	// ErrEmptyBounds is returned when the requested bounds have zero or
	// negative width or height.
	ErrEmptyBounds = errors.New("mask: bounds must not be empty")
	// ErrTooLarge is returned when the requested bounds exceed the
	// MaxPixels budget. It is returned before any full-size work buffer
	// is allocated.
	ErrTooLarge = errors.New("mask: bounds exceed the pixel budget")
	// ErrInvalidKernel is returned when a structuring element has
	// non-positive dimensions or no points.
	ErrInvalidKernel = errors.New("mask: kernel must have positive dimensions and at least one point")
)

// checkBounds validates r against the empty and budget rules using 64-bit
// arithmetic so that extreme rectangles cannot overflow the width/height
// multiplication. It returns the width and height on success.
func checkBounds(r image.Rectangle) (w, h int, err error) {
	dw := int64(r.Max.X) - int64(r.Min.X)
	dh := int64(r.Max.Y) - int64(r.Min.Y)
	if dw <= 0 || dh <= 0 {
		return 0, 0, ErrEmptyBounds
	}
	if dw > MaxPixels || dh > MaxPixels || dw*dh > MaxPixels {
		return 0, 0, ErrTooLarge
	}
	return int(dw), int(dh), nil
}

// Binarize returns a binary mask of img thresholded at level, covering
// exactly img.Bounds(). It is equivalent to BinarizeRegion(img, level,
// img.Bounds()).
func Binarize(img image.Image, level uint8) (*image.Gray, error) {
	if img == nil {
		return nil, ErrNilImage
	}
	return BinarizeRegion(img, level, img.Bounds())
}

// BinarizeRegion returns a binary mask of img thresholded at level over the
// rectangle r. The result is an *image.Gray with bounds exactly equal to r,
// preserving the global coordinates of the source: the output pixel at global
// position (x, y) is derived from the source pixel at the same global
// position (x, y), regardless of where either image's origin lies.
//
// Conversion rules:
//   - Samples whose global position falls outside img.Bounds() are
//     background (0x00).
//   - A fully transparent pixel (alpha == 0 after conversion to RGBA) is
//     always background (0x00), regardless of level.
//   - Otherwise the perceived luminance (0.3R + 0.6G + 0.1B of the
//     premultiplied RGBA value) is compared against level: values greater
//     than or equal to level become foreground (0xFF), the rest background
//     (0x00).
//
// The source image is never modified.
func BinarizeRegion(img image.Image, level uint8, r image.Rectangle) (*image.Gray, error) {
	if img == nil {
		return nil, ErrNilImage
	}
	w, h, err := checkBounds(r)
	if err != nil {
		return nil, err
	}

	dst := image.NewGray(r)
	sb := img.Bounds()

	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			gy := r.Min.Y + y
			for x := 0; x < w; x++ {
				gx := r.Min.X + x
				pos := y*dst.Stride + x

				if !(image.Point{X: gx, Y: gy}).In(sb) {
					dst.Pix[pos] = 0x00
					continue
				}

				c := color.RGBAModel.Convert(img.At(gx, gy)).(color.RGBA)
				if c.A == 0 {
					dst.Pix[pos] = 0x00
					continue
				}

				if uint8(util.Rank(c)) >= level {
					dst.Pix[pos] = 0xFF
				} else {
					dst.Pix[pos] = 0x00
				}
			}
		}
	})

	return dst, nil
}

// ToRGBA returns an RGBA copy of the mask m with the same bounds, mapping
// each pixel to {v, v, v, 0xFF} where v is the mask value. The result never
// shares memory with m.
func ToRGBA(m *image.Gray) (*image.RGBA, error) {
	if m == nil {
		return nil, ErrNilMask
	}
	w, h, err := checkBounds(m.Bounds())
	if err != nil {
		return nil, err
	}

	bounds := m.Bounds()
	dst := image.NewRGBA(bounds)

	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			for x := 0; x < w; x++ {
				v := m.Pix[y*m.Stride+x]
				pos := y*dst.Stride + x*4
				dst.Pix[pos+0] = v
				dst.Pix[pos+1] = v
				dst.Pix[pos+2] = v
				dst.Pix[pos+3] = 0xFF
			}
		}
	})

	return dst, nil
}
