package images

import (
	"image"

	"github.com/disintegration/imaging"
)

type Dimensions struct {
	Width  int
	Height int
}

type ResizeOptions struct {
	MaxWidth  int
	MaxHeight int
}

func Open(inputPath string) (image.Image, error) {
	return imaging.Open(inputPath)
}

// Resize an image according to supplied options
func Resize(src image.Image, opts ResizeOptions) image.Image {
	// Get source dimensions, calculate new dimensions
	srcWidth := src.Bounds().Dx()
	srcHeight := src.Bounds().Dy()
	original := Dimensions{Width: srcWidth, Height: srcHeight}
	desired := Dimensions{Width: opts.MaxWidth, Height: opts.MaxHeight}
	resized := calculateDimensions(original, desired)

	// NearestNeighbor has best perf, but looks horrible at low res
	filter := imaging.NearestNeighbor
	if resized.Width < 1000 || resized.Height < 1000 {
		filter = imaging.CatmullRom
	}

	// Resize the image
	resizedImg := imaging.Resize(src, resized.Width, resized.Height, filter)

	return resizedImg
}

func Save(src image.Image, outputPath string) error {
	return imaging.Save(src, outputPath)
}

// calculateDimensions determines the new dimensions based on the maximum constraints
func calculateDimensions(source Dimensions, max Dimensions) Dimensions {
	// If only one dimension is specified, use it as the constraint
	if max.Width == 0 {
		max.Width = source.Width
	}
	if max.Height == 0 {
		max.Height = source.Height
	}

	// Calculate ratios
	widthRatio := float64(max.Width) / float64(source.Width)
	heightRatio := float64(max.Height) / float64(source.Height)

	// Use the smaller ratio to ensure both dimensions fit within max bounds
	ratio := widthRatio
	if heightRatio < widthRatio {
		ratio = heightRatio
	}

	// Calculate new dimensions
	newWidth := int(float64(source.Width) * ratio)
	newHeight := int(float64(source.Height) * ratio)

	// Handle case where new dimensions might be 0
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}

	return Dimensions{
		Height: newHeight,
		Width:  newWidth,
	}
}
