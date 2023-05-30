package rgb

import (
	"image"
	"image/color"
)

type RGB struct {
	// Pix holds the image's pixels, in R, G, B order. The pixel at
	// (x, y) starts at Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*3].
	Pix []byte
	// Stride is the Pix stride (in bytes) between vertically adjacent pixels.
	Stride int
	// Rect is the image's bounds.
	Rect image.Rectangle
}

func (p *RGB) ColorModel() color.Model { return color.RGBAModel }

func (p *RGB) Bounds() image.Rectangle { return p.Rect }

func (p *RGB) At(x, y int) color.Color {
	if !(image.Point{X: x, Y: y}.In(p.Rect)) {
		return color.RGBA{}
	}
	i := (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
	s := p.Pix[i : i+3 : i+3] // Small cap improves performance, see https://golang.org/issue/27857
	return color.RGBA{R: s[0], G: s[1], B: s[2], A: 0}
}

func NewRGB(data []byte, width, height int) *RGB {
	return &RGB{
		Pix:    data,
		Stride: len(data) / height,
		Rect: image.Rectangle{
			Min: image.Point{X: 0, Y: 0},
			Max: image.Point{X: width, Y: height},
		},
	}
}
