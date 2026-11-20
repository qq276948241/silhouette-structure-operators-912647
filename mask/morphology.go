package mask

import (
	"image"

	"github.com/anthonynsimon/bild/parallel"
)

// binarizeCutoff is the value at or above which a grayscale mask pixel is
// treated as foreground by the morphological operations.
const binarizeCutoff = 0x80

// Dilate returns the morphological dilation of the mask m by the kernel k,
// repeated iterations times. See morph for the exact semantics.
func Dilate(m *image.Gray, k Kernel, iterations int) (*image.Gray, error) {
	return morph(m, k, iterations, true)
}

// Erode returns the morphological erosion of the mask m by the kernel k,
// repeated iterations times. See morph for the exact semantics.
func Erode(m *image.Gray, k Kernel, iterations int) (*image.Gray, error) {
	return morph(m, k, iterations, false)
}

// Open returns the morphological opening of m by k: an erosion followed by
// a dilation, each repeated iterations times. Opening is idempotent:
// opening an already opened mask leaves it unchanged.
func Open(m *image.Gray, k Kernel, iterations int) (*image.Gray, error) {
	eroded, err := Erode(m, k, iterations)
	if err != nil {
		return nil, err
	}
	return Dilate(eroded, k, iterations)
}

// Close returns the morphological closing of m by k: a dilation followed by
// an erosion, each repeated iterations times. Closing is idempotent:
// closing an already closed mask leaves it unchanged.
func Close(m *image.Gray, k Kernel, iterations int) (*image.Gray, error) {
	dilated, err := Dilate(m, k, iterations)
	if err != nil {
		return nil, err
	}
	return Erode(dilated, k, iterations)
}

// morph applies a flat binary dilation (local maximum) or erosion (local
// minimum) of m by k, repeated iterations times.
//
// Fixed rules:
//   - Input normalization: m is first binarized; pixels >= 0x80 are
//     foreground (1), the rest background (0). Non-binary inputs are
//     therefore handled deterministically.
//   - Out-of-bounds samples take a fixed value per operation: background
//     (0) for dilation and foreground (1) for erosion. Together with the
//     reflected-kernel convention below this makes dilation and erosion an
//     adjunction, so Open and Close are idempotent for any kernel.
//   - Dilation applies the kernel reflected about its anchor (the standard
//     dilation convention); erosion applies it as-is. For symmetric kernels
//     (odd-sized Rectangle and Cross) the reflection changes nothing.
//   - iterations <= 0 is normalized to 1.
//   - The result has exactly the bounds of m, preserving global
//     coordinates; m itself is never modified.
//
// Work buffers are allocated once per call and reused across iterations;
// rectangular and cross kernels are evaluated as separable 1-D sliding
// passes, so even large kernels never allocate a full-image array per step.
func morph(m *image.Gray, k Kernel, iterations int, dilate bool) (*image.Gray, error) {
	if m == nil {
		return nil, ErrNilMask
	}
	if len(k.offsets) == 0 {
		return nil, ErrInvalidKernel
	}
	w, h, err := checkBounds(m.Bounds())
	if err != nil {
		return nil, err
	}
	if iterations < 1 {
		iterations = 1
	}

	n := w * h
	a := make([]uint8, n)
	b := make([]uint8, n)

	// Normalize the input into a 0/1 plane.
	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			for x := 0; x < w; x++ {
				if m.Pix[y*m.Stride+x] >= binarizeCutoff {
					a[y*w+x] = 1
				}
			}
		}
	})

	switch k.kind {
	case kindRect:
		for i := 0; i < iterations; i++ {
			axisPass(a, b, w, h, k.w, anchorFor(k.w, k.anchor.X, dilate), true, dilate, false)
			axisPass(b, a, w, h, k.h, anchorFor(k.h, k.anchor.Y, dilate), false, dilate, false)
		}
	case kindCross:
		for i := 0; i < iterations; i++ {
			// Dilation/erosion by a cross equals the max/min of the
			// horizontal-line and vertical-line results over the same input.
			axisPass(a, b, w, h, k.w, anchorFor(k.w, k.anchor.X, dilate), true, dilate, false)
			axisPass(a, b, w, h, k.h, anchorFor(k.h, k.anchor.Y, dilate), false, dilate, true)
			a, b = b, a
		}
	default: // kindCustom
		offsets := k.offsets
		if dilate {
			// Reflect the kernel about its anchor.
			reflected := make([]image.Point, len(offsets))
			for i, o := range offsets {
				reflected[i] = image.Point{X: -o.X, Y: -o.Y}
			}
			offsets = reflected
		}
		for i := 0; i < iterations; i++ {
			customPass(a, b, w, h, offsets, dilate)
			a, b = b, a
		}
	}

	dst := image.NewGray(m.Bounds())
	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			for x := 0; x < w; x++ {
				if a[y*w+x] != 0 {
					dst.Pix[y*dst.Stride+x] = 0xFF
				}
			}
		}
	})

	return dst, nil
}

// anchorFor returns the sliding-window anchor for one axis. Dilation uses
// the reflected kernel, so its anchor is klen-1-anchor; erosion uses the
// anchor as-is.
func anchorFor(klen, anchor int, dilate bool) int {
	if dilate {
		return klen - 1 - anchor
	}
	return anchor
}

// axisPass applies a 1-D sliding maximum (dilate) or minimum (erode) of
// length klen along rows (horizontal) or columns (vertical) of src into
// dst. When combine is true the result is merged into dst (max for
// dilation, min for erosion) instead of overwriting it, which lets a cross
// kernel evaluate both axes against the same input.
func axisPass(src, dst []uint8, w, h, klen, anchor int, horizontal, dilate, combine bool) {
	if horizontal {
		parallel.Line(h, func(start, end int) {
			dq := make([]int, 0, klen)
			for y := start; y < end; y++ {
				slide1D(src, dst, w, y*w, y*w, 1, klen, anchor, dilate, combine, dq)
			}
		})
		return
	}
	parallel.Line(w, func(start, end int) {
		dq := make([]int, 0, klen)
		for x := start; x < end; x++ {
			slide1D(src, dst, h, x, x, w, klen, anchor, dilate, combine, dq)
		}
	})
}

// slide1D computes, for n samples starting at srcOff/dstOff with the given
// stride, the sliding window maximum (dilate) or minimum (erode) over the
// window [i-anchor, i-anchor+klen-1]. Out-of-range samples take the fixed
// value 0 for dilation and 1 for erosion. dq is scratch space for the
// monotone deque and is reset per call.
func slide1D(src, dst []uint8, n, srcOff, dstOff, stride, klen, anchor int, dilate, combine bool, dq []int) {
	dq = dq[:0]

	push := func(j int) {
		v := src[srcOff+j*stride]
		if dilate {
			for len(dq) > 0 && src[srcOff+dq[len(dq)-1]*stride] <= v {
				dq = dq[:len(dq)-1]
			}
		} else {
			for len(dq) > 0 && src[srcOff+dq[len(dq)-1]*stride] >= v {
				dq = dq[:len(dq)-1]
			}
		}
		dq = append(dq, j)
	}

	// Seed the deque with the in-range samples of the first window, whose
	// right edge is klen-1-anchor.
	if firstHi := klen - 1 - anchor; firstHi > 0 {
		for j := 0; j < firstHi && j < n; j++ {
			push(j)
		}
	}

	for i := 0; i < n; i++ {
		lo := i - anchor
		hi := lo + klen - 1

		if hi >= 0 && hi < n {
			push(hi)
		}

		for len(dq) > 0 && dq[0] < lo {
			dq = dq[1:]
		}

		var v uint8
		if len(dq) == 0 {
			// The window covers no in-range samples; use the fixed
			// out-of-bounds value.
			if dilate {
				v = 0
			} else {
				v = 1
			}
		} else {
			v = src[srcOff+dq[0]*stride]
		}

		di := dstOff + i*stride
		if combine {
			if dilate {
				if v > dst[di] {
					dst[di] = v
				}
			} else {
				if v < dst[di] {
					dst[di] = v
				}
			}
		} else {
			dst[di] = v
		}
	}
}

// customPass applies one iteration of an arbitrary structuring element by
// direct evaluation of its offsets. Out-of-bounds samples take the fixed
// value 0 for dilation and 1 for erosion.
func customPass(src, dst []uint8, w, h int, offsets []image.Point, dilate bool) {
	parallel.Line(h, func(start, end int) {
		for y := start; y < end; y++ {
			for x := 0; x < w; x++ {
				var v uint8
				if dilate {
					v = 0
				} else {
					v = 1
				}
				for _, off := range offsets {
					sx := x + off.X
					sy := y + off.Y
					var s uint8
					if sx >= 0 && sx < w && sy >= 0 && sy < h {
						s = src[sy*w+sx]
					} else if !dilate {
						s = 1
					}
					if dilate {
						if s > v {
							v = s
						}
					} else {
						if s < v {
							v = s
						}
					}
				}
				dst[y*w+x] = v
			}
		}
	})
}
