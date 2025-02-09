package test

import (
	"testing"

	"github.com/disintegration/imaging"
)

const maxWidth = 600
const maxHeight = 600

func BenchmarkResizeNearestNeighbor(b *testing.B) {
	filter := imaging.NearestNeighbor
	srcImage, err := imaging.Open("data/ambience.jpg")
	if err != nil {
		panic(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = imaging.Resize(srcImage, maxWidth, maxHeight, filter)
	}
}

func BenchmarkResizeCatmullRom(b *testing.B) {
	filter := imaging.CatmullRom
	srcImage, err := imaging.Open("data/ambience.jpg")
	if err != nil {
		panic(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = imaging.Resize(srcImage, maxWidth, maxHeight, filter)
	}
}

func BenchmarkResizeLinear(b *testing.B) {
	filter := imaging.Linear
	srcImage, err := imaging.Open("data/ambience.jpg")
	if err != nil {
		panic(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = imaging.Resize(srcImage, maxWidth, maxHeight, filter)
	}
}

func BenchmarkResizeLanczos(b *testing.B) {
	filter := imaging.Lanczos
	srcImage, err := imaging.Open("data/ambience.jpg")
	if err != nil {
		panic(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = imaging.Resize(srcImage, maxWidth, maxHeight, filter)
	}
}
