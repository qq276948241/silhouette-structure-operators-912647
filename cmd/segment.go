package cmd

import (
	"errors"
	"fmt"
	"image"

	"github.com/anthonynsimon/bild/imgio"
	"github.com/anthonynsimon/bild/mask"
	"github.com/anthonynsimon/bild/segment"
	"github.com/spf13/cobra"
)

// errUnknownKernel is thrown when an unknown structuring element name is provided.
var errUnknownKernel = errors.New("unknown kernel, options: rect, cross")

func threshold() *cobra.Command {
	var level uint8

	var cmd = &cobra.Command{
		Use:     "threshold",
		Short:   "segment an image by a threshold",
		Args:    cobra.ExactArgs(2),
		Example: "threshold --level 200 input.jpg output.jpg",
		Run: func(cmd *cobra.Command, args []string) {
			fin := args[0]
			fout := args[1]

			apply(fin, fout, func(img image.Image) (image.Image, error) {
				return segment.Threshold(img, level), nil
			})
		}}

	cmd.Flags().Uint8VarP(&level, "level", "l", 128, "the level at which the segmenting threshold will be crossed")

	return cmd
}

func binarize() *cobra.Command {
	var level uint8

	var cmd = &cobra.Command{
		Use:     "mask",
		Short:   "create a binary mask of an image by thresholding its luminance",
		Args:    cobra.ExactArgs(2),
		Example: "mask --level 128 input.jpg mask.png",
		Run: func(cmd *cobra.Command, args []string) {
			fin := args[0]
			fout := args[1]

			apply(fin, fout, func(img image.Image) (image.Image, error) {
				return mask.Binarize(img, level)
			})
		}}

	cmd.Flags().Uint8VarP(&level, "level", "l", 128, "the luminance level at or above which a pixel becomes mask foreground")

	return cmd
}

// resolveKernel builds a structuring element from the CLI kernel flags.
func resolveKernel(name string, width, height int) (mask.Kernel, error) {
	switch name {
	case "rect":
		return mask.Rectangle(width, height)
	case "cross":
		return mask.Cross(width, height)
	default:
		return mask.Kernel{}, errUnknownKernel
	}
}

func morphology(name string) *cobra.Command {
	var level uint8
	var kernel string
	var width, height, iterations int

	var cmd = &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s the bright regions of an image's binary mask", name),
		Long: fmt.Sprintf(`%s binarizes the input image by luminance threshold and applies a
morphological %s with the selected structuring element.

Out-of-bounds samples count as background for dilation and foreground for
erosion; iterations <= 0 are treated as 1.`, name, name),
		Args:    cobra.ExactArgs(2),
		Example: fmt.Sprintf("%s --kernel rect --width 3 --height 3 --iterations 1 input.jpg output.png", name),
		Run: func(cmd *cobra.Command, args []string) {
			fin := args[0]
			fout := args[1]

			apply(fin, fout, func(img image.Image) (image.Image, error) {
				k, err := resolveKernel(kernel, width, height)
				if err != nil {
					return nil, err
				}
				m, err := mask.Binarize(img, level)
				if err != nil {
					return nil, err
				}
				switch name {
				case "dilate":
					return mask.Dilate(m, k, iterations)
				case "erode":
					return mask.Erode(m, k, iterations)
				case "open":
					return mask.Open(m, k, iterations)
				case "close":
					return mask.Close(m, k, iterations)
				}
				return nil, fmt.Errorf("unknown morphology operation %q", name)
			})
		}}

	cmd.Flags().Uint8VarP(&level, "level", "l", 128, "the luminance level used to binarize the input")
	cmd.Flags().StringVarP(&kernel, "kernel", "k", "rect", "structuring element shape: rect, cross")
	cmd.Flags().IntVarP(&width, "width", "w", 3, "structuring element width")
	cmd.Flags().IntVarP(&height, "height", "H", 3, "structuring element height")
	cmd.Flags().IntVarP(&iterations, "iterations", "i", 1, "number of times the operation is applied")

	return cmd
}

func composite() *cobra.Command {
	var maskPath string
	var level uint8
	var opacity, strength float64

	var cmd = &cobra.Command{
		Use:   "composite",
		Short: "overlay a foreground image onto a background through a binary mask",
		Long: `composite overlays the foreground onto the background, restricting the
foreground's coverage to the foreground regions of the mask.

All three images are aligned by their global coordinates, so they may
differ in size and origin; the result keeps the background's bounds.
The mask image is binarized by luminance at --level.`,
		Args:    cobra.ExactArgs(3),
		Example: "composite --mask mask.png --opacity 0.8 --strength 1 background.jpg foreground.png output.png",
		Run: func(cmd *cobra.Command, args []string) {
			bg, err := imgio.Open(args[0])
			exitIfNotNil(err)

			fg, err := imgio.Open(args[1])
			exitIfNotNil(err)

			maskImg, err := imgio.Open(maskPath)
			exitIfNotNil(err)

			m, err := mask.Binarize(maskImg, level)
			exitIfNotNil(err)

			result, err := mask.Composite(bg, fg, m, opacity, strength)
			exitIfNotNil(err)

			encoder := resolveEncoder(args[2], imgio.PNGEncoder())
			exitIfNotNil(imgio.Save(args[2], result, encoder))
		}}

	cmd.Flags().StringVarP(&maskPath, "mask", "m", "", "path to the mask image (required)")
	cmd.Flags().Uint8VarP(&level, "level", "l", 128, "the luminance level used to binarize the mask image")
	cmd.Flags().Float64VarP(&opacity, "opacity", "o", 1, "foreground opacity multiplier in the range 0 to 1")
	cmd.Flags().Float64VarP(&strength, "strength", "s", 1, "mask strength multiplier in the range 0 to 1")
	_ = cmd.MarkFlagRequired("mask")

	return cmd
}

func createSegment() *cobra.Command {
	var segmentCmd = &cobra.Command{
		Use:   "segment",
		Short: "segment an image using the specified method",
	}

	segmentCmd.AddCommand(threshold())
	segmentCmd.AddCommand(binarize())
	segmentCmd.AddCommand(morphology("dilate"))
	segmentCmd.AddCommand(morphology("erode"))
	segmentCmd.AddCommand(morphology("open"))
	segmentCmd.AddCommand(morphology("close"))
	segmentCmd.AddCommand(composite())

	return segmentCmd
}
