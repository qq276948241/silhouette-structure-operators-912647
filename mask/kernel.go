package mask

import "image"

// kernelKind identifies the internal shape of a Kernel so that separable
// shapes can take a fast path that does not allocate a full-image buffer
// per iteration.
type kernelKind uint8

const (
	kindCustom kernelKind = iota
	kindRect
	kindCross
)

// Kernel is a structuring element for the morphological operations Dilate,
// Erode, Open and Close. Construct one with Rectangle, Cross or NewKernel.
//
// The anchor is the kernel point that is aligned with the output pixel.
// For the built-in shapes the anchor is at (width/2, height/2) using integer
// division, so for even-sized dimensions the anchor sits one step right of
// (and below) the geometric center.
type Kernel struct {
	kind    kernelKind
	w, h    int
	anchor  image.Point
	offsets []image.Point
}

// Rectangle returns a solid rectangular structuring element of width w and
// height h. Both dimensions must be >= 1. The anchor is (w/2, h/2).
func Rectangle(w, h int) (Kernel, error) {
	if w < 1 || h < 1 {
		return Kernel{}, ErrInvalidKernel
	}
	anchor := image.Point{X: w / 2, Y: h / 2}
	offsets := make([]image.Point, 0, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			offsets = append(offsets, image.Point{X: x - anchor.X, Y: y - anchor.Y})
		}
	}
	return Kernel{kind: kindRect, w: w, h: h, anchor: anchor, offsets: offsets}, nil
}

// Cross returns a plus-shaped structuring element: the full horizontal row
// and the full vertical column that meet at the anchor (w/2, h/2). Both
// dimensions must be >= 1.
func Cross(w, h int) (Kernel, error) {
	if w < 1 || h < 1 {
		return Kernel{}, ErrInvalidKernel
	}
	anchor := image.Point{X: w / 2, Y: h / 2}
	offsets := make([]image.Point, 0, w+h-1)
	for x := 0; x < w; x++ {
		offsets = append(offsets, image.Point{X: x - anchor.X, Y: 0})
	}
	for y := 0; y < h; y++ {
		if y == anchor.Y {
			continue // already covered by the horizontal row
		}
		offsets = append(offsets, image.Point{X: 0, Y: y - anchor.Y})
	}
	return Kernel{kind: kindCross, w: w, h: h, anchor: anchor, offsets: offsets}, nil
}

// NewKernel returns a custom structuring element from the given points,
// expressed in kernel coordinates, and the anchor point within them.
// points must contain at least one element.
func NewKernel(points []image.Point, anchor image.Point) (Kernel, error) {
	if len(points) == 0 {
		return Kernel{}, ErrInvalidKernel
	}
	offsets := make([]image.Point, len(points))
	for i, p := range points {
		offsets[i] = p.Sub(anchor)
	}
	return Kernel{kind: kindCustom, anchor: anchor, offsets: offsets}, nil
}

// Anchor returns the kernel's anchor point in kernel coordinates.
func (k Kernel) Anchor() image.Point {
	return k.anchor
}

// Offsets returns a copy of the kernel's points relative to the anchor.
// The result may be reused to build further kernels with NewKernel.
func (k Kernel) Offsets() []image.Point {
	out := make([]image.Point, len(k.offsets))
	copy(out, k.offsets)
	return out
}
