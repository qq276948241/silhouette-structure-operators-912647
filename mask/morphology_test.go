package mask

import (
	"errors"
	"image"
	"runtime"
	"sync"
	"testing"

	"github.com/anthonynsimon/bild/util"
)

// grayFromStrings builds a mask from ASCII art with its origin at min:
// '#' is foreground (0xFF), anything else is background (0x00).
func grayFromStrings(min image.Point, rows []string) *image.Gray {
	h := len(rows)
	w := len(rows[0])
	m := image.NewGray(image.Rect(min.X, min.Y, min.X+w, min.Y+h))
	for y, row := range rows {
		for x, c := range row {
			if c == '#' {
				m.Pix[y*m.Stride+x] = 0xFF
			}
		}
	}
	return m
}

// randomGray returns a deterministic pseudo-random binary mask over r.
func randomGray(r image.Rectangle, seed uint32) *image.Gray {
	m := image.NewGray(r)
	s := seed
	for i := range m.Pix {
		s = s*1664525 + 1013904223
		if (s>>24)%5 < 2 {
			m.Pix[i] = 0xFF
		}
	}
	return m
}

// naiveMorph is a deliberately simple per-pixel reference implementation of
// the documented semantics: normalize at 0x80, dilation uses the reflected
// kernel with out-of-bounds = 0, erosion uses the kernel as-is with
// out-of-bounds = 1.
func naiveMorph(m *image.Gray, k Kernel, iterations int, dilate bool) *image.Gray {
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()

	src := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if m.Pix[y*m.Stride+x] >= binarizeCutoff {
				src[y*w+x] = 1
			}
		}
	}

	offsets := k.Offsets()
	if dilate {
		for i, o := range offsets {
			offsets[i] = image.Point{X: -o.X, Y: -o.Y}
		}
	}

	if iterations < 1 {
		iterations = 1
	}
	for it := 0; it < iterations; it++ {
		dst := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var v uint8
				if !dilate {
					v = 1
				}
				for _, o := range offsets {
					sx := x + o.X
					sy := y + o.Y
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
		src = dst
	}

	out := image.NewGray(b)
	for i, v := range src {
		if v != 0 {
			out.Pix[i] = 0xFF
		}
	}
	return out
}

func mustRectangle(t *testing.T, w, h int) Kernel {
	t.Helper()
	k, err := Rectangle(w, h)
	if err != nil {
		t.Fatalf("unexpected kernel error: %v", err)
	}
	return k
}

func mustCross(t *testing.T, w, h int) Kernel {
	t.Helper()
	k, err := Cross(w, h)
	if err != nil {
		t.Fatalf("unexpected kernel error: %v", err)
	}
	return k
}

func mustNewKernel(t *testing.T, points []image.Point, anchor image.Point) Kernel {
	t.Helper()
	k, err := NewKernel(points, anchor)
	if err != nil {
		t.Fatalf("unexpected kernel error: %v", err)
	}
	return k
}

func testKernels(t *testing.T) []Kernel {
	diamond := mustNewKernel(t, []image.Point{
		{X: 1, Y: 0},
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1},
		{X: 1, Y: 2},
	}, image.Point{X: 1, Y: 1})
	asymmetric := mustNewKernel(t, []image.Point{
		{X: 0, Y: 0}, {X: 2, Y: 1},
	}, image.Point{X: 0, Y: 0})
	return []Kernel{
		mustRectangle(t, 1, 1),
		mustRectangle(t, 3, 3),
		mustRectangle(t, 4, 2), // even width
		mustRectangle(t, 2, 6), // even both
		mustRectangle(t, 7, 1), // horizontal line
		mustCross(t, 3, 3),
		mustCross(t, 4, 4), // even cross
		mustCross(t, 1, 5), // vertical line
		diamond,
		asymmetric,
	}
}

func testMasks() []*image.Gray {
	return []*image.Gray{
		// holes in a solid field
		grayFromStrings(image.Point{}, []string{
			"#########",
			"#########",
			"###.###.#",
			"#########",
			"##....###",
			"#########",
			"#########",
		}),
		// isolated points
		grayFromStrings(image.Point{}, []string{
			".........",
			"..#.....#",
			".........",
			".....#...",
			".........",
			".#.......",
			".........",
		}),
		// thin lines
		grayFromStrings(image.Point{}, []string{
			".........",
			".........",
			"#########",
			".........",
			"....#....",
			"....#....",
			"....#....",
		}),
		// all black
		grayFromStrings(image.Point{}, []string{
			".....",
			".....",
			".....",
		}),
		// all white
		grayFromStrings(image.Point{}, []string{
			"#####",
			"#####",
			"#####",
		}),
		// deterministic random, origin zero
		randomGray(image.Rect(0, 0, 13, 9), 42),
		// deterministic random, non-zero origin
		randomGray(image.Rect(-4, 3, 9, 12), 1337),
	}
}

func TestMorphAgainstNaive(t *testing.T) {
	kernels := testKernels(t)
	masks := testMasks()

	for ki, k := range kernels {
		for mi, m := range masks {
			for _, iterations := range []int{1, 2, 3} {
				gotD, err := Dilate(m, k, iterations)
				if err != nil {
					t.Fatalf("kernel %d mask %d: dilate error: %v", ki, mi, err)
				}
				wantD := naiveMorph(m, k, iterations, true)
				if !util.GrayImageEqual(gotD, wantD) {
					t.Errorf("kernel %d mask %d iterations %d: dilate mismatch\ngot:  %v\nwant: %v", ki, mi, iterations, gotD, wantD)
				}

				gotE, err := Erode(m, k, iterations)
				if err != nil {
					t.Fatalf("kernel %d mask %d: erode error: %v", ki, mi, err)
				}
				wantE := naiveMorph(m, k, iterations, false)
				if !util.GrayImageEqual(gotE, wantE) {
					t.Errorf("kernel %d mask %d iterations %d: erode mismatch\ngot:  %v\nwant: %v", ki, mi, iterations, gotE, wantE)
				}
			}
		}
	}
}

func TestMorphPreservesBounds(t *testing.T) {
	m := randomGray(image.Rect(-7, 5, 3, 20), 7)
	k := mustRectangle(t, 4, 2)

	for name, fn := range map[string]func(*image.Gray, Kernel, int) (*image.Gray, error){
		"dilate": Dilate,
		"erode":  Erode,
		"open":   Open,
		"close":  Close,
	} {
		out, err := fn(m, k, 2)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if !out.Bounds().Eq(m.Bounds()) {
			t.Errorf("%s: expected bounds %v, got %v", name, m.Bounds(), out.Bounds())
		}
	}
}

func TestOpenCloseIdempotent(t *testing.T) {
	kernels := testKernels(t)
	masks := testMasks()

	for ki, k := range kernels {
		for mi, m := range masks {
			open1, err := Open(m, k, 1)
			if err != nil {
				t.Fatalf("kernel %d mask %d: open error: %v", ki, mi, err)
			}
			open2, err := Open(open1, k, 1)
			if err != nil {
				t.Fatalf("kernel %d mask %d: open error: %v", ki, mi, err)
			}
			if !util.GrayImageEqual(open1, open2) {
				t.Errorf("kernel %d mask %d: open is not idempotent", ki, mi)
			}

			close1, err := Close(m, k, 1)
			if err != nil {
				t.Fatalf("kernel %d mask %d: close error: %v", ki, mi, err)
			}
			close2, err := Close(close1, k, 1)
			if err != nil {
				t.Fatalf("kernel %d mask %d: close error: %v", ki, mi, err)
			}
			if !util.GrayImageEqual(close1, close2) {
				t.Errorf("kernel %d mask %d: close is not idempotent", ki, mi)
			}
		}
	}
}

func TestOpenRemovesIsolatedPoints(t *testing.T) {
	m := grayFromStrings(image.Point{}, []string{
		".........",
		".........",
		"..#......",
		".........",
		".....#...",
		".........",
		".........",
	})
	k := mustRectangle(t, 3, 3)

	opened, err := Open(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range opened.Pix {
		if v != 0 {
			t.Fatalf("open did not remove isolated point at index %d", i)
		}
	}

	// Points far from every border survive closing unchanged: dilation
	// grows them to a 3x3 block and erosion shrinks the block back.
	closed, err := Close(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !util.GrayImageEqual(closed, m) {
		t.Errorf("close changed isolated points\ngot:  %v\nwant: %v", closed, m)
	}
}

func TestCloseFillsHoles(t *testing.T) {
	m := grayFromStrings(image.Point{}, []string{
		"#########",
		"#########",
		"#########",
		"####.####",
		"#########",
		"#########",
		"#########",
	})
	k := mustRectangle(t, 3, 3)

	closed, err := Close(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range closed.Pix {
		if v != 0xFF {
			t.Fatalf("close did not fill the hole at index %d", i)
		}
	}

	// Opening keeps the hole: erosion grows it, dilation restores it.
	opened, err := Open(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opened.Pix[3*opened.Stride+4] != 0x00 {
		t.Error("open filled the hole")
	}
}

func TestOpenRemovesThinLines(t *testing.T) {
	m := grayFromStrings(image.Point{}, []string{
		".........",
		".........",
		"#########",
		".........",
		".........",
	})
	k := mustRectangle(t, 3, 3)

	opened, err := Open(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range opened.Pix {
		if v != 0 {
			t.Fatalf("open did not remove the thin line at index %d", i)
		}
	}
}

func TestMorphAllBlackAllWhite(t *testing.T) {
	black := grayFromStrings(image.Point{}, []string{"....", "....", "...."})
	white := grayFromStrings(image.Point{}, []string{"####", "####", "####"})
	k := mustRectangle(t, 3, 3)

	// Dilating an all-black mask keeps it black (out-of-bounds is 0).
	got, err := Dilate(black, k, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !util.GrayImageEqual(got, black) {
		t.Error("dilate changed an all-black mask")
	}

	// Eroding an all-white mask keeps it white (out-of-bounds is 1).
	got, err = Erode(white, k, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !util.GrayImageEqual(got, white) {
		t.Error("erode changed an all-white mask")
	}

	// Open/Close are stable on constant masks.
	for _, m := range []*image.Gray{black, white} {
		o, err := Open(m, k, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !util.GrayImageEqual(o, m) {
			t.Error("open changed a constant mask")
		}
		c, err := Close(m, k, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !util.GrayImageEqual(c, m) {
			t.Error("close changed a constant mask")
		}
	}
}

func TestMorphNormalizesNonBinaryInput(t *testing.T) {
	// 0x7F is below the 0x80 cutoff, 0x80 and above are foreground.
	m := &image.Gray{
		Rect:   image.Rect(0, 0, 4, 1),
		Stride: 4,
		Pix:    []uint8{0x7F, 0x80, 0x00, 0xC8},
	}
	binary := &image.Gray{
		Rect:   image.Rect(0, 0, 4, 1),
		Stride: 4,
		Pix:    []uint8{0x00, 0xFF, 0x00, 0xFF},
	}
	k := mustRectangle(t, 3, 1)

	gotD, err := Dilate(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantD, err := Dilate(binary, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !util.GrayImageEqual(gotD, wantD) {
		t.Errorf("dilate did not normalize the non-binary input\ngot:  %v\nwant: %v", gotD, wantD)
	}

	gotE, err := Erode(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantE, err := Erode(binary, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !util.GrayImageEqual(gotE, wantE) {
		t.Errorf("erode did not normalize the non-binary input\ngot:  %v\nwant: %v", gotE, wantE)
	}
}

func TestMorphIterationsNormalized(t *testing.T) {
	m := randomGray(image.Rect(0, 0, 9, 9), 5)
	k := mustCross(t, 3, 3)

	want, err := Dilate(m, k, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, it := range []int{0, -1, -100} {
		got, err := Dilate(m, k, it)
		if err != nil {
			t.Fatalf("iterations %d: unexpected error: %v", it, err)
		}
		if !util.GrayImageEqual(got, want) {
			t.Errorf("iterations %d was not normalized to 1", it)
		}
	}
}

func TestMorphErrors(t *testing.T) {
	m := grayFromStrings(image.Point{}, []string{"#.#", "...", "#.#"})
	k := mustRectangle(t, 3, 3)

	if _, err := Dilate(nil, k, 1); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}
	if _, err := Erode(nil, k, 1); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}
	if _, err := Open(nil, k, 1); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}
	if _, err := Close(nil, k, 1); !errors.Is(err, ErrNilMask) {
		t.Errorf("expected ErrNilMask, got %v", err)
	}

	empty := &image.Gray{Rect: image.Rect(0, 0, 0, 4)}
	if _, err := Dilate(empty, k, 1); !errors.Is(err, ErrEmptyBounds) {
		t.Errorf("expected ErrEmptyBounds, got %v", err)
	}

	huge := &image.Gray{Rect: image.Rect(0, 0, MaxPixels+1, 2)}
	if _, err := Dilate(huge, k, 1); !errors.Is(err, ErrTooLarge) {
		t.Errorf("expected ErrTooLarge, got %v", err)
	}

	if _, err := Dilate(m, Kernel{}, 1); !errors.Is(err, ErrInvalidKernel) {
		t.Errorf("expected ErrInvalidKernel, got %v", err)
	}
	if _, err := Rectangle(0, 3); !errors.Is(err, ErrInvalidKernel) {
		t.Errorf("expected ErrInvalidKernel, got %v", err)
	}
	if _, err := Rectangle(3, -1); !errors.Is(err, ErrInvalidKernel) {
		t.Errorf("expected ErrInvalidKernel, got %v", err)
	}
	if _, err := Cross(0, 0); !errors.Is(err, ErrInvalidKernel) {
		t.Errorf("expected ErrInvalidKernel, got %v", err)
	}
	if _, err := NewKernel(nil, image.Point{}); !errors.Is(err, ErrInvalidKernel) {
		t.Errorf("expected ErrInvalidKernel, got %v", err)
	}
}

func TestMorphDoesNotMutateInput(t *testing.T) {
	m := randomGray(image.Rect(2, 2, 12, 12), 99)
	snapshot := make([]uint8, len(m.Pix))
	copy(snapshot, m.Pix)

	k := mustRectangle(t, 3, 3)
	for _, fn := range []func(*image.Gray, Kernel, int) (*image.Gray, error){Dilate, Erode, Open, Close} {
		if _, err := fn(m, k, 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	for i := range m.Pix {
		if m.Pix[i] != snapshot[i] {
			t.Fatalf("input mask modified at index %d", i)
		}
	}
}

func TestMorphRepeatedCalls(t *testing.T) {
	m := randomGray(image.Rect(-2, -2, 10, 10), 123)
	k := mustRectangle(t, 4, 2)

	first, err := Open(m, k, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snapshot := make([]uint8, len(first.Pix))
	copy(snapshot, first.Pix)

	for i := 0; i < 4; i++ {
		next, err := Open(m, k, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !util.GrayImageEqual(first, next) {
			t.Fatalf("call %d: results differ between calls", i)
		}
	}
	for i := range first.Pix {
		if first.Pix[i] != snapshot[i] {
			t.Fatalf("first result mutated by a later call at index %d", i)
		}
	}
}

func TestMorphConcurrent(t *testing.T) {
	m := randomGray(image.Rect(-4, -4, 16, 16), 2024)
	k := mustRectangle(t, 5, 3)

	reference, err := Close(m, k, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Close(m, k, 2)
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

// TestMorphNoPerIterationAlloc ensures that iterating with a large kernel
// does not allocate a full-image buffer per iteration: the extra memory
// used by 8 iterations over 1 must stay well below one image plane.
func TestMorphNoPerIterationAlloc(t *testing.T) {
	w, h := 512, 512
	m := randomGray(image.Rect(0, 0, w, h), 17)
	k := mustRectangle(t, 31, 31)

	// Warm up goroutine pools and runtime structures.
	if _, err := Dilate(m, k, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var ms runtime.MemStats

	runtime.ReadMemStats(&ms)
	before1 := ms.TotalAlloc
	if _, err := Dilate(m, k, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runtime.ReadMemStats(&ms)
	alloc1 := ms.TotalAlloc - before1

	runtime.ReadMemStats(&ms)
	before8 := ms.TotalAlloc
	if _, err := Dilate(m, k, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runtime.ReadMemStats(&ms)
	alloc8 := ms.TotalAlloc - before8

	plane := uint64(w * h)
	if alloc8 > alloc1+plane {
		t.Errorf("8 iterations allocated %d bytes over 1 iteration (%d), exceeds one plane (%d): per-iteration full-image allocation suspected",
			alloc8-alloc1, alloc1, plane)
	}
}
