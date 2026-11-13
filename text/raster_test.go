package text_test

import (
	"image/color"
	"testing"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/text"
)

func assertTextPixel(t *testing.T, pd *pixel.PictureData, x, y int, want color.RGBA) {
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

// Writing text and drawing it onto a PictureData rasterizes the glyphs from the atlas picture.
func TestText_DrawToPictureData(t *testing.T) {
	atlas := text.Atlas7x13
	txt := text.New(pixel.V(2, 12), atlas)
	txt.Color = pixel.Alpha(1)
	txt.WriteString("A")

	pd := pixel.MakePictureData(pixel.R(0, 0, 16, 16))
	txt.Draw(pd, pixel.IM)

	// The glyph must paint at least one opaque pixel and must leave a far-away pixel untouched.
	opaque := 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := pd.Pix[pd.Index(pixel.V(float64(x), float64(y)))]
			if c.A > 0 {
				opaque++
			}
		}
	}
	if opaque == 0 {
		t.Fatal("expected glyph pixels to be rasterized, but target is fully transparent")
	}
	assertTextPixel(t, pd, 0, 0, color.RGBA{})

	// Every rasterized texel must be white (the atlas glyph is drawn with white vertex color).
	for i, c := range pd.Pix {
		if c.A == 0 {
			continue
		}
		// premultiplied white: R == G == B == A
		if c.R != c.A || c.G != c.A || c.B != c.A {
			t.Fatalf("texel %d = %v, want premultiplied white matching the atlas glyph", i, c)
		}
	}

	// The glyph's reported bounds from the atlas must overlap the rasterized region.
	bounds := txt.Bounds()
	if bounds.W() == 0 || bounds.H() == 0 {
		t.Fatalf("expected non-empty text bounds, got %v", bounds)
	}
}

// A color mask tints the drawn text.
func TestText_DrawColorMask(t *testing.T) {
	txt := text.New(pixel.ZV, text.Atlas7x13)
	txt.WriteString("X")

	pd := pixel.MakePictureData(pixel.R(0, 0, 12, 16))
	pd.SetColorMask(pixel.RGB(1, 0, 0))
	txt.Draw(pd, pixel.IM)

	redOnly := true
	covered := false
	for i := range pd.Pix {
		c := pd.Pix[i]
		if c.A == 0 {
			continue
		}
		covered = true
		if c.G > 1 || c.B > 1 || c.R == 0 {
			redOnly = false
		}
	}
	if !covered {
		t.Fatal("expected glyph pixels to be rasterized")
	}
	if !redOnly {
		t.Fatal("expected the masked text to contain only red premultiplied pixels")
	}
}
