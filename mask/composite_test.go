package mask

import (
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/anthonynsimon/bild/util"
)

// solidRGBA returns a uniform opaque RGBA image over r.
func solidRGBA(r image.Rectangle, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(r)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}
	return img
}

// solidGrayMask returns a uniform mask over r.
func solidGrayMask(r image.Rectangle, v uint8) *image.Gray {
	m := image.NewGray(r)
	for i := range m.Pix {
		m.Pix[i] = v
	}
	return m
}

func TestCompositePixels(t *testing.T) {
	bg := func() image.Image {
		return solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{100, 100, 100, 255})
	}
	fg := func() image.Image {
		return solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{200, 0, 0, 255})
	}

	cases := []struct {
		name        string
		bg          image.Image
		fg          image.Image
		mask        *image.Gray
		opacity     float64
		strength    float64
		expectedPix []uint8
	}{
		{
			name:        "full coverage",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     1,
			strength:    1,
			expectedPix: []uint8{200, 0, 0, 255, 200, 0, 0, 255},
		},
		{
			name:        "zero mask keeps background",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0x00),
			opacity:     1,
			strength:    1,
			expectedPix: []uint8{100, 100, 100, 255, 100, 100, 100, 255},
		},
		{
			name:        "half mask value",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 128),
			opacity:     1,
			strength:    1,
			expectedPix: []uint8{150, 50, 50, 255, 150, 50, 50, 255},
		},
		{
			name:        "half opacity",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     0.5,
			strength:    1,
			expectedPix: []uint8{150, 50, 50, 255, 150, 50, 50, 255},
		},
		{
			name:        "half mask strength",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     1,
			strength:    0.5,
			expectedPix: []uint8{150, 50, 50, 255, 150, 50, 50, 255},
		},
		{
			name:        "zero opacity",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     0,
			strength:    1,
			expectedPix: []uint8{100, 100, 100, 255, 100, 100, 100, 255},
		},
		{
			name:        "zero strength",
			bg:          bg(),
			fg:          fg(),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     1,
			strength:    0,
			expectedPix: []uint8{100, 100, 100, 255, 100, 100, 100, 255},
		},
		{
			name:        "rounding half away from zero",
			bg:          solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{0, 0, 0, 255}),
			fg:          solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{255, 255, 255, 255}),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     0.5,
			strength:    1,
			expectedPix: []uint8{128, 128, 128, 255, 128, 128, 128, 255},
		},
		{
			name:        "semi transparent foreground",
			bg:          bg(),
			fg:          solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{200, 0, 0, 128}),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     1,
			strength:    1,
			expectedPix: []uint8{150, 50, 50, 255, 150, 50, 50, 255},
		},
		{
			name:        "semi transparent background and foreground",
			bg:          solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{100, 100, 100, 128}),
			fg:          solidRGBA(image.Rect(0, 0, 2, 1), color.RGBA{200, 0, 0, 128}),
			mask:        solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF),
			opacity:     1,
			strength:    1,
			expectedPix: []uint8{150, 50, 50, 192, 150, 50, 50, 192},
		},
	}

	for _, c := range cases {
		out, err := Composite(c.bg, c.fg, c.mask, c.opacity, c.strength)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		expected := &image.RGBA{
			Rect:   image.Rect(0, 0, 2, 1),
			Stride: 2 * 4,
			Pix:    c.expectedPix,
		}
		if !util.RGBAImageEqual(out, expected) {
			t.Errorf("%s: expected: %v actual: %v", c.name, expected, out)
		}
	}
}

func TestCompositeLocalMask(t *testing.T) {
	bg := solidRGBA(image.Rect(0, 0, 4, 1), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(0, 0, 4, 1), color.RGBA{200, 0, 0, 255})
	// The mask only covers the two middle pixels.
	m := solidGrayMask(image.Rect(1, 0, 3, 1), 0xFF)

	out, err := Composite(bg, fg, m, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := &image.RGBA{
		Rect:   image.Rect(0, 0, 4, 1),
		Stride: 4 * 4,
		Pix: []uint8{
			100, 100, 100, 255,
			200, 0, 0, 255,
			200, 0, 0, 255,
			100, 100, 100, 255,
		},
	}
	if !util.RGBAImageEqual(out, expected) {
		t.Errorf("expected: %v actual: %v", expected, out)
	}
}

func TestCompositeGlobalAlignment(t *testing.T) {
	// bg and fg have different bounds and non-zero origins; pixels are
	// aligned by global coordinates.
	bg := solidRGBA(image.Rect(0, 0, 2, 2), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(1, 1, 3, 3), color.RGBA{200, 0, 0, 255})
	m := solidGrayMask(image.Rect(0, 0, 2, 2), 0xFF)

	out, err := Composite(bg, fg, m, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Bounds().Eq(bg.Bounds()) {
		t.Fatalf("expected bounds %v, got %v", bg.Bounds(), out.Bounds())
	}
	expected := &image.RGBA{
		Rect:   image.Rect(0, 0, 2, 2),
		Stride: 2 * 4,
		Pix: []uint8{
			100, 100, 100, 255, 100, 100, 100, 255, // fg does not cover (0,0) or (1,0)
			100, 100, 100, 255, 200, 0, 0, 255, // fg covers global (1,1) only
		},
	}
	if !util.RGBAImageEqual(out, expected) {
		t.Errorf("expected: %v actual: %v", expected, out)
	}
}

func TestCompositeNonZeroOriginBackground(t *testing.T) {
	bg := solidRGBA(image.Rect(5, 5, 7, 7), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(6, 6, 8, 8), color.RGBA{0, 0, 200, 255})
	m := solidGrayMask(image.Rect(5, 5, 7, 7), 0xFF)

	out, err := Composite(bg, fg, m, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Bounds().Eq(image.Rect(5, 5, 7, 7)) {
		t.Fatalf("expected bounds (5,5)-(7,7), got %v", out.Bounds())
	}
	expected := &image.RGBA{
		Rect:   image.Rect(5, 5, 7, 7),
		Stride: 2 * 4,
		Pix: []uint8{
			100, 100, 100, 255, 100, 100, 100, 255,
			100, 100, 100, 255, 0, 0, 200, 255,
		},
	}
	if !util.RGBAImageEqual(out, expected) {
		t.Errorf("expected: %v actual: %v", expected, out)
	}
}

func TestCompositeNonRGBAInputs(t *testing.T) {
	bg := image.NewGray(image.Rect(0, 0, 2, 1))
	bg.Pix[0] = 100
	bg.Pix[1] = 100

	fg := &image.NRGBA{
		Rect:   image.Rect(0, 0, 2, 1),
		Stride: 2 * 4,
		Pix: []uint8{
			200, 0, 0, 255,
			200, 0, 0, 255,
		},
	}
	m := solidGrayMask(image.Rect(0, 0, 2, 1), 0xFF)

	out, err := Composite(bg, fg, m, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := &image.RGBA{
		Rect:   image.Rect(0, 0, 2, 1),
		Stride: 2 * 4,
		Pix:    []uint8{200, 0, 0, 255, 200, 0, 0, 255},
	}
	if !util.RGBAImageEqual(out, expected) {
		t.Errorf("expected: %v actual: %v", expected, out)
	}
}

func TestCompositeDoesNotShareMemory(t *testing.T) {
	bg := solidRGBA(image.Rect(0, 0, 2, 2), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(0, 0, 2, 2), color.RGBA{200, 0, 0, 255})
	m := solidGrayMask(image.Rect(0, 0, 2, 2), 0xFF)

	out, err := Composite(bg, fg, m, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if &out.Pix[0] == &bg.Pix[0] || &out.Pix[0] == &fg.Pix[0] {
		t.Fatal("composite result shares its Pix slice with an input")
	}

	// Mutating the result must not affect the inputs.
	for i := range out.Pix {
		out.Pix[i] = 0x11
	}
	for i := 0; i < len(bg.Pix); i += 4 {
		if bg.Pix[i] != 100 || fg.Pix[i] != 200 {
			t.Fatal("input pixel data changed after mutating the result")
		}
	}
}

func TestCompositeErrors(t *testing.T) {
	img := solidRGBA(image.Rect(0, 0, 2, 2), color.RGBA{1, 2, 3, 255})
	m := solidGrayMask(image.Rect(0, 0, 2, 2), 0xFF)

	if _, err := Composite(nil, img, m, 1, 1); !errors.Is(err, ErrNilImage) {
		t.Errorf("expected ErrNilImage, got %v", err)
	}
	if _, err := Composite(img, nil, m, 1, 1); !errors.Is(err, ErrNilImage) {
		t.Errorf("expected ErrNilImage, got %v", err)
	}
	if _, err := Composite(img, img, nil, 1, 1); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}

	empty := &image.RGBA{Rect: image.Rect(0, 0, 0, 0)}
	if _, err := Composite(empty, img, m, 1, 1); !errors.Is(err, ErrEmptyBounds) {
		t.Errorf("expected ErrEmptyBounds, got %v", err)
	}

	// image.Uniform reports bounds of +/-1e9 per axis, over budget.
	if _, err := Composite(image.NewUniform(color.Black), img, m, 1, 1); !errors.Is(err, ErrTooLarge) {
		t.Errorf("expected ErrTooLarge, got %v", err)
	}
}

func TestCompositeRepeatedCalls(t *testing.T) {
	bg := solidRGBA(image.Rect(3, 3, 9, 9), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(3, 3, 9, 9), color.RGBA{200, 0, 0, 200})
	m := randomGray(image.Rect(3, 3, 9, 9), 11)

	first, err := Composite(bg, fg, m, 0.7, 0.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snapshot := make([]uint8, len(first.Pix))
	copy(snapshot, first.Pix)

	for i := 0; i < 4; i++ {
		next, err := Composite(bg, fg, m, 0.7, 0.9)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !util.RGBAImageEqual(first, next) {
			t.Fatalf("call %d: results differ between calls", i)
		}
	}
	for i := range first.Pix {
		if first.Pix[i] != snapshot[i] {
			t.Fatalf("first result mutated by a later call at index %d", i)
		}
	}
}

func TestCompositeConcurrent(t *testing.T) {
	bg := solidRGBA(image.Rect(-2, -2, 10, 10), color.RGBA{100, 100, 100, 255})
	fg := solidRGBA(image.Rect(-2, -2, 10, 10), color.RGBA{200, 0, 0, 200})
	m := randomGray(image.Rect(-2, -2, 10, 10), 77)

	reference, err := Composite(bg, fg, m, 0.8, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Composite(bg, fg, m, 0.8, 1)
			if err != nil {
				errs <- err
				return
			}
			if !util.RGBAImageEqual(reference, got) {
				errs <- errors.New("concurrent result differs from reference")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
