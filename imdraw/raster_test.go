package imdraw_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/imdraw"
)

func assertIMDPixel(t *testing.T, pd *pixel.PictureData, x, y int, want color.RGBA) {
	t.Helper()
	got := pd.Pix[pd.Index(pixel.V(float64(x), float64(y)))]
	diff := func(a, b uint8) uint8 {
		if a > b {
			return a - b
		}
		return b - a
	}
	if diff(got.R, want.R) > 1 || diff(got.G, want.G) > 1 || diff(got.B, want.B) > 1 || diff(got.A, want.A) > 1 {
		t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
	}
}

// A filled IMDraw rectangle rasterizes onto a PictureData with its push color.
func TestIMDraw_FillRectangle(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 8, 8))
	imd := imdraw.New(nil)

	imd.Color = pixel.RGB(1, 0, 0)
	imd.Push(pixel.V(1, 1), pixel.V(6, 6))
	imd.Rectangle(0)
	imd.Draw(pd)

	// an interior pixel of the filled rectangle
	assertIMDPixel(t, pd, 2, 2, color.RGBA{R: 255, A: 255})
	// a pixel outside the rectangle stays transparent
	assertIMDPixel(t, pd, 0, 0, color.RGBA{})
}

// A filled IMDraw circle is rasterized and the color mask is applied.
func TestIMDraw_FillCircleMasked(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 12, 12))
	imd := imdraw.New(nil)

	// green geometry, masked down to the green channel via a green mask
	pd.SetColorMask(pixel.RGB(0, 1, 0))
	imd.Color = pixel.Alpha(1)
	imd.Push(pixel.V(6, 6))
	imd.Circle(4, 0)
	imd.Draw(pd)

	// center of the circle is covered
	assertIMDPixel(t, pd, 6, 6, color.RGBA{G: 255, A: 255})
	// well outside the radius remains empty
	assertIMDPixel(t, pd, 0, 0, color.RGBA{})
}

// IMDraw can draw a textured polygon onto a PictureData.
func TestIMDraw_TexturedPolygon(t *testing.T) {
	srcImg := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			srcImg.SetRGBA(x, y, color.RGBA{R: 10, G: 200, B: 30, A: 255})
		}
	}
	src := pixel.PictureDataFromImage(srcImg)

	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 4))
	imd := imdraw.New(src)

	imd.Color = pixel.Alpha(1)
	imd.Intensity = 1
	imd.Picture = pixel.V(2, 2)
	imd.Push(pixel.V(0, 0), pixel.V(4, 0), pixel.V(0, 4))
	imd.Polygon(0)
	imd.Draw(pd)

	// the covered lower-left region shows the texture color
	assertIMDPixel(t, pd, 0, 0, color.RGBA{R: 10, G: 200, B: 30, A: 255})
}
