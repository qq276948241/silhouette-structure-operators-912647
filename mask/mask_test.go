package mask

import (
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/anthonynsimon/bild/util"
)

func TestBinarize(t *testing.T) {
	cases := []struct {
		name     string
		level    uint8
		img      image.Image
		expected *image.Gray
	}{
		{
			// Note: a 0x80 gray pixel ranks 127.999... in float64 and
			// truncates to 127, matching segment.Threshold's behavior.
			name:  "rgba origin zero",
			level: 128,
			img: &image.RGBA{
				Rect:   image.Rect(0, 0, 2, 2),
				Stride: 2 * 4,
				Pix: []uint8{
					0x80, 0x80, 0x80, 0xFF, 0x40, 0x40, 0x40, 0xFF,
					0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00,
				},
			},
			expected: &image.Gray{
				Rect:   image.Rect(0, 0, 2, 2),
				Stride: 2,
				Pix: []uint8{
					0x00, 0x00,
					0xFF, 0x00, // last pixel is fully transparent -> background
				},
			},
		},
		{
			name:  "rgba non-zero origin keeps global coordinates",
			level: 128,
			img: &image.RGBA{
				Rect:   image.Rect(3, 5, 5, 7),
				Stride: 2 * 4,
				Pix: []uint8{
					0x80, 0x80, 0x80, 0xFF, 0x40, 0x40, 0x40, 0xFF,
					0xFF, 0xFF, 0xFF, 0xFF, 0x10, 0x10, 0x10, 0xFF,
				},
			},
			expected: &image.Gray{
				Rect:   image.Rect(3, 5, 5, 7),
				Stride: 2,
				Pix: []uint8{
					0x00, 0x00,
					0xFF, 0x00,
				},
			},
		},
		{
			name:  "gray input",
			level: 100,
			img: &image.Gray{
				Rect:   image.Rect(-2, -1, 1, 1),
				Stride: 3,
				Pix: []uint8{
					0x00, 0x64, 0xC8,
					0x63, 0xFF, 0x80,
				},
			},
			expected: &image.Gray{
				Rect:   image.Rect(-2, -1, 1, 1),
				Stride: 3,
				Pix: []uint8{
					0x00, 0xFF, 0xFF,
					0x00, 0xFF, 0xFF,
				},
			},
		},
		{
			name:  "nrgba input with zero alpha",
			level: 128,
			img: &image.NRGBA{
				Rect:   image.Rect(0, 0, 3, 1),
				Stride: 3 * 4,
				Pix: []uint8{
					0xC8, 0xC8, 0xC8, 0xFF, // opaque bright -> foreground
					0xFF, 0xFF, 0xFF, 0x00, // bright but transparent -> background
					0x64, 0x64, 0x64, 0xFF, // opaque dark -> background
				},
			},
			expected: &image.Gray{
				Rect:   image.Rect(0, 0, 3, 1),
				Stride: 3,
				Pix:    []uint8{0xFF, 0x00, 0x00},
			},
		},
		{
			name:  "luminance weights",
			level: 128,
			img: &image.RGBA{
				Rect:   image.Rect(0, 0, 2, 1),
				Stride: 2 * 4,
				Pix: []uint8{
					0xFF, 0x00, 0x00, 0xFF, // rank 0.3*255 = 76 -> background
					0x00, 0xFF, 0x00, 0xFF, // rank 0.6*255 = 153 -> foreground
				},
			},
			expected: &image.Gray{
				Rect:   image.Rect(0, 0, 2, 1),
				Stride: 2,
				Pix:    []uint8{0x00, 0xFF},
			},
		},
	}

	for _, c := range cases {
		actual, err := Binarize(c.img, c.level)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if !util.GrayImageEqual(actual, c.expected) {
			t.Errorf("%s: expected: %v actual: %v", c.name, c.expected, actual)
		}
	}
}

func TestBinarizeRegion(t *testing.T) {
	img := &image.RGBA{
		Rect:   image.Rect(10, 10, 12, 12),
		Stride: 2 * 4,
		Pix: []uint8{
			0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0xFF,
			0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		},
	}

	cases := []struct {
		name     string
		region   image.Rectangle
		expected *image.Gray
	}{
		{
			name:   "same bounds",
			region: image.Rect(10, 10, 12, 12),
			expected: &image.Gray{
				Rect:   image.Rect(10, 10, 12, 12),
				Stride: 2,
				Pix: []uint8{
					0xFF, 0x00,
					0x00, 0xFF,
				},
			},
		},
		{
			name:   "sub-region keeps global offset",
			region: image.Rect(11, 10, 12, 11),
			expected: &image.Gray{
				Rect:   image.Rect(11, 10, 12, 11),
				Stride: 1,
				Pix:    []uint8{0x00},
			},
		},
		{
			name:   "region extending outside image is background",
			region: image.Rect(9, 9, 13, 11),
			expected: &image.Gray{
				Rect:   image.Rect(9, 9, 13, 11),
				Stride: 4,
				Pix: []uint8{
					0x00, 0x00, 0x00, 0x00,
					0x00, 0xFF, 0x00, 0x00,
				},
			},
		},
		{
			name:   "region fully outside image",
			region: image.Rect(0, 0, 2, 2),
			expected: &image.Gray{
				Rect:   image.Rect(0, 0, 2, 2),
				Stride: 2,
				Pix: []uint8{
					0x00, 0x00,
					0x00, 0x00,
				},
			},
		},
	}

	for _, c := range cases {
		actual, err := BinarizeRegion(img, 128, c.region)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if !util.GrayImageEqual(actual, c.expected) {
			t.Errorf("%s: expected: %v actual: %v", c.name, c.expected, actual)
		}
	}
}

func TestBinarizeDoesNotMutateSource(t *testing.T) {
	img := &image.RGBA{
		Rect:   image.Rect(0, 0, 2, 2),
		Stride: 2 * 4,
		Pix: []uint8{
			0x80, 0x80, 0x80, 0xFF, 0x40, 0x40, 0x40, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00,
		},
	}
	snapshot := make([]uint8, len(img.Pix))
	copy(snapshot, img.Pix)

	if _, err := Binarize(img, 128); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range img.Pix {
		if img.Pix[i] != snapshot[i] {
			t.Fatalf("source pixel data modified at index %d", i)
		}
	}
}

func TestBinarizeErrors(t *testing.T) {
	img := &image.Gray{
		Rect:   image.Rect(0, 0, 2, 2),
		Stride: 2,
		Pix:    []uint8{0xFF, 0x00, 0x00, 0xFF},
	}

	cases := []struct {
		name   string
		img    image.Image
		region image.Rectangle
		target error
	}{
		{name: "nil image", img: nil, region: image.Rect(0, 0, 1, 1), target: ErrNilImage},
		{name: "empty width", img: img, region: image.Rect(0, 0, 0, 5), target: ErrEmptyBounds},
		{name: "empty height", img: img, region: image.Rect(0, 0, 5, 0), target: ErrEmptyBounds},
		{name: "inverted rect", img: img, region: image.Rectangle{Min: image.Point{X: 5, Y: 5}, Max: image.Point{X: 1, Y: 1}}, target: ErrEmptyBounds},
		// image.Uniform reports bounds of +/-1e9 per axis, over budget.
		{name: "over budget image bounds", img: image.NewUniform(color.White), region: image.Rectangle{}, target: ErrTooLarge},
		// A rectangle wider than int32 range must not overflow the
		// width*height multiplication before the budget check.
		{name: "extreme rect no overflow", img: img, region: image.Rectangle{Min: image.Point{X: -1 << 31, Y: -1 << 31}, Max: image.Point{X: 1 << 31, Y: 1 << 31}}, target: ErrTooLarge},
		{name: "over budget region", img: img, region: image.Rect(0, 0, MaxPixels+1, 1), target: ErrTooLarge},
	}

	for _, c := range cases {
		var err error
		if c.name == "over budget image bounds" {
			_, err = Binarize(c.img, 128)
		} else if c.img == nil {
			_, err = Binarize(nil, 128)
		} else {
			_, err = BinarizeRegion(c.img, 128, c.region)
		}
		if !errors.Is(err, c.target) {
			t.Errorf("%s: expected error %v, got %v", c.name, c.target, err)
		}
	}
}

func TestBinarizeRepeatedCalls(t *testing.T) {
	img := &image.NRGBA{
		Rect:   image.Rect(4, 4, 8, 8),
		Stride: 4 * 4,
		Pix:    make([]uint8, 4*4*4),
	}
	for i := 0; i < len(img.Pix); i += 4 {
		v := uint8((i * 37) % 256)
		img.Pix[i+0] = v
		img.Pix[i+1] = v
		img.Pix[i+2] = v
		img.Pix[i+3] = 0xFF
	}

	first, err := Binarize(img, 128)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snapshot := make([]uint8, len(first.Pix))
	copy(snapshot, first.Pix)

	for i := 0; i < 4; i++ {
		next, err := Binarize(img, 128)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !util.GrayImageEqual(first, next) {
			t.Fatalf("call %d: results differ between calls", i)
		}
	}
	// The first result must not have been touched by later calls.
	for i := range first.Pix {
		if first.Pix[i] != snapshot[i] {
			t.Fatalf("first result mutated by a later call at index %d", i)
		}
	}
}

func TestBinarizeConcurrent(t *testing.T) {
	img := &image.RGBA{
		Rect:   image.Rect(-3, -3, 5, 5),
		Stride: 8 * 4,
		Pix:    make([]uint8, 8*8*4),
	}
	for i := 0; i < len(img.Pix); i += 4 {
		v := uint8((i * 13) % 256)
		img.Pix[i+0] = v
		img.Pix[i+1] = 255 - v
		img.Pix[i+2] = v / 2
		img.Pix[i+3] = 0xFF
	}

	reference, err := Binarize(img, 128)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Binarize(img, 128)
			if err != nil {
				errs <- err
				return
			}
			if !util.GrayImageEqual(reference, got) {
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

func TestToRGBA(t *testing.T) {
	m := &image.Gray{
		Rect:   image.Rect(2, 3, 4, 4),
		Stride: 2,
		Pix:    []uint8{0xFF, 0x00},
	}

	actual, err := ToRGBA(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := &image.RGBA{
		Rect:   image.Rect(2, 3, 4, 4),
		Stride: 2 * 4,
		Pix: []uint8{
			0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0xFF,
		},
	}
	if !util.RGBAImageEqual(actual, expected) {
		t.Errorf("expected: %v actual: %v", expected, actual)
	}

	// The result must not share memory with the input.
	actual.Pix[0] = 0x11
	if m.Pix[0] != 0xFF {
		t.Error("ToRGBA result shares memory with the input mask")
	}

	if _, err := ToRGBA(nil); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}
	if _, err := ToRGBA(&image.Gray{Rect: image.Rect(0, 0, 0, 2)}); !errors.Is(err, ErrEmptyBounds) {
		t.Errorf("expected ErrEmptyBounds, got %v", err)
	}
}
