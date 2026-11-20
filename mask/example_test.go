package mask_test

import (
	"fmt"
	"image"

	"github.com/anthonynsimon/bild/mask"
)

// Rectangle builds a solid 3x2 structuring element. Its anchor is at
// (3/2, 2/2) = (1, 1), so the offsets cover x in [-1, 1] and y in [-1, 0].
func ExampleRectangle() {
	k, err := mask.Rectangle(3, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println("anchor:", k.Anchor())
	fmt.Println("offsets:", k.Offsets())
	// Output:
	// anchor: (1,1)
	// offsets: [(-1,-1) (0,-1) (1,-1) (-1,0) (0,0) (1,0)]
}

// Cross builds a plus-shaped 3x3 structuring element that can be reused
// across any number of morphological operations.
func ExampleCross() {
	k, err := mask.Cross(3, 3)
	if err != nil {
		panic(err)
	}
	fmt.Println("anchor:", k.Anchor())
	fmt.Println("offsets:", k.Offsets())
	// Output:
	// anchor: (1,1)
	// offsets: [(-1,0) (0,0) (1,0) (0,-1) (0,1)]
}

// NewKernel builds a custom diamond-shaped structuring element from
// explicit points and an anchor.
func ExampleNewKernel() {
	diamond := []image.Point{
		{X: 1, Y: 0},
		{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1},
		{X: 1, Y: 2},
	}
	k, err := mask.NewKernel(diamond, image.Point{X: 1, Y: 1})
	if err != nil {
		panic(err)
	}
	fmt.Println("anchor:", k.Anchor())
	fmt.Println("offsets:", k.Offsets())
	// Output:
	// anchor: (1,1)
	// offsets: [(0,-1) (-1,0) (0,0) (1,0) (0,1)]
}
