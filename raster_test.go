package pixel_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/faiface/pixel"
)

// channelTolerance is the accepted per-channel difference (in 8-bit units), as required.
const channelTolerance = 1

func assertPixelRGBA(t *testing.T, pd *pixel.PictureData, x, y int, want color.RGBA) {
	t.Helper()
	got := pd.Pix[pd.Index(pixel.V(float64(x), float64(y)))]
	if abs8(got.R, want.R) > channelTolerance ||
		abs8(got.G, want.G) > channelTolerance ||
		abs8(got.B, want.B) > channelTolerance ||
		abs8(got.A, want.A) > channelTolerance {
		t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
	}
}

func assertPixelColor(t *testing.T, pd *pixel.PictureData, x, y int, want pixel.RGBA) {
	t.Helper()
	assertPixelRGBA(t, pd, x, y, color.RGBA{
		R: uint8(math.Round(want.R * 255)),
		G: uint8(math.Round(want.G * 255)),
		B: uint8(math.Round(want.B * 255)),
		A: uint8(math.Round(want.A * 255)),
	})
}

func abs8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func solidPicture(size int, c color.RGBA) *pixel.PictureData {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return pixel.PictureDataFromImage(img)
}

// quadPoints are the six vertices (two CCW triangles) covering a rectangle r.
func quadPoints(r pixel.Rect) [6]pixel.Vec {
	return [6]pixel.Vec{
		{X: r.Min.X, Y: r.Min.Y},
		{X: r.Max.X, Y: r.Min.Y},
		{X: r.Max.X, Y: r.Max.Y},
		{X: r.Min.X, Y: r.Min.Y},
		{X: r.Max.X, Y: r.Max.Y},
		{X: r.Min.X, Y: r.Max.Y},
	}
}

// solidQuadColor builds two triangles covering r with a uniform vertex color.
func solidQuadColor(r pixel.Rect, c pixel.RGBA) *pixel.TrianglesData {
	tri := pixel.MakeTrianglesData(6)
	for i, pos := range quadPoints(r) {
		(*tri)[i].Position = pos
		(*tri)[i].Color = c
	}
	return tri
}

// A newly created PictureData must default to the identity matrix and a white color mask.
func TestPictureData_NewDefaults(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 4))

	// white mask must not change a drawn color
	tri := pixel.MakeTrianglesData(3)
	(*tri)[0].Position = pixel.V(0, 0)
	(*tri)[1].Position = pixel.V(4, 0)
	(*tri)[2].Position = pixel.V(0, 4)
	for i := range *tri {
		(*tri)[i].Color = pixel.RGB(1, 0, 0)
	}
	pd.MakeTriangles(tri).Draw()

	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 255, A: 255})
}

// A Sprite drawn 1:1 onto a PictureData rasterizes every covered pixel at its exact source color.
func TestPictureData_SpriteSolid(t *testing.T) {
	src := solidPicture(2, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 4))

	sprite := pixel.NewSprite(src, src.Bounds())
	sprite.Draw(pd, pixel.IM.Moved(pixel.V(2, 2)))

	// covered pixels keep the source color
	assertPixelRGBA(t, pd, 1, 1, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	assertPixelRGBA(t, pd, 2, 2, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	// untouched pixels stay transparent
	assertPixelRGBA(t, pd, 3, 3, color.RGBA{})
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{})
}

// Vertex colors modulate a white textured sprite and work on untextured triangles.
func TestPictureData_VertexColor(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 2, 2))

	// single triangle covering half of the target, solid green
	tri := pixel.MakeTrianglesData(3)
	(*tri)[0].Position = pixel.V(0, 0)
	(*tri)[1].Position = pixel.V(2, 0)
	(*tri)[2].Position = pixel.V(0, 2)
	for i := range *tri {
		(*tri)[i].Color = pixel.RGB(0, 1, 0)
	}
	pd.MakeTriangles(tri).Draw()

	assertPixelRGBA(t, pd, 0, 0, color.RGBA{G: 255, A: 255})
	// opposite corner must remain empty (triangle only covers the lower-left half)
	assertPixelRGBA(t, pd, 1, 1, color.RGBA{})
}

// Texture coordinates and intensity blend between vertex color and the picture.
func TestPictureData_Intensity(t *testing.T) {
	src := solidPicture(2, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 2, 2))

	tri := quadPoints(pixel.R(0, 0, 2, 2))
	td := pixel.MakeTrianglesData(6)
	for i, pos := range tri {
		(*td)[i].Position = pos
		(*td)[i].Color = pixel.RGB(1, 0, 0)
		(*td)[i].Picture = pos
		(*td)[i].Intensity = 0.25
	}
	tt := pd.MakeTriangles(td)
	pp := pd.MakePicture(src)
	pp.Draw(tt)

	// 0.75*red + 0.25*red*white = red; red channel full, others zero
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 255, A: 255})
	assertPixelRGBA(t, pd, 1, 1, color.RGBA{R: 255, A: 255})
}

// With full intensity the picture color replaces the vertex color.
func TestPictureData_IntensityFull(t *testing.T) {
	src := solidPicture(1, color.RGBA{R: 30, G: 60, B: 90, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 1, 1))

	td := pixel.MakeTrianglesData(6)
	for i, pos := range quadPoints(pixel.R(0, 0, 1, 1)) {
		(*td)[i].Position = pos
		(*td)[i].Color = pixel.Alpha(1)
		(*td)[i].Picture = pixel.V(0.5, 0.5)
		(*td)[i].Intensity = 1
	}
	pd.MakePicture(src).Draw(pd.MakeTriangles(td))

	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 30, G: 60, B: 90, A: 255})
}

// A color mask multiplies the fragment color.
func TestPictureData_ColorMask(t *testing.T) {
	src := solidPicture(2, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 2, 2))
	pd.SetColorMask(pixel.RGB(1, 0, 0))

	sprite := pixel.NewSprite(src, src.Bounds())
	sprite.Draw(pd, pixel.IM)

	// an interior pixel center is fully covered; white texture masked by red yields red
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 255, A: 255})
}

// ClipRect discards fragments outside of the clip rectangle; target bounds clip everything else.
func TestPictureData_ClipRectAndBounds(t *testing.T) {
	src := solidPicture(4, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 4))

	clip := pixel.R(1, 1, 3, 3)
	tri := pixel.MakeTrianglesData(6)
	frame := src.Bounds()
	corners := [6]pixel.Vec{
		{X: frame.Min.X, Y: frame.Min.Y},
		{X: frame.Max.X, Y: frame.Min.Y},
		{X: frame.Max.X, Y: frame.Max.Y},
		{X: frame.Min.X, Y: frame.Min.Y},
		{X: frame.Max.X, Y: frame.Max.Y},
		{X: frame.Min.X, Y: frame.Max.Y},
	}
	for i, pos := range corners {
		(*tri)[i].Position = pos
		(*tri)[i].Color = pixel.Alpha(1)
		(*tri)[i].Picture = pos
		(*tri)[i].Intensity = 1
		(*tri)[i].ClipRect = clip
		(*tri)[i].IsClipped = true
	}
	tt := pd.MakeTriangles(tri)
	pd.MakePicture(src).Draw(tt)

	// inside the clip
	assertPixelRGBA(t, pd, 1, 1, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	assertPixelRGBA(t, pd, 2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	// outside the clip
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{})
	assertPixelRGBA(t, pd, 3, 3, color.RGBA{})
}

// Fragments outside the target's pixel rectangle are clipped away.
func TestPictureData_OutOfBoundsClip(t *testing.T) {
	src := solidPicture(4, color.RGBA{R: 255, A: 255})
	pd := pixel.MakePictureData(pixel.R(0, 0, 2, 2))

	sprite := pixel.NewSprite(src, src.Bounds())
	// shift the sprite so only one pixel overlaps the target
	sprite.Draw(pd, pixel.IM.Moved(pixel.V(-1, -1)))

	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 255, A: 255})
	assertPixelRGBA(t, pd, 1, 1, color.RGBA{})
}

// Semi-transparent drawing uses ComposeOver premultiplied blending.
func TestPictureData_ComposeOver(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 1, 1))

	// opaque blue background
	pd.MakeTriangles(solidQuadColor(pixel.R(0, 0, 1, 1), pixel.RGB(0, 0, 1))).Draw()

	// 50% white on top: result = 0.5*white + 0.5*blue
	pd.MakeTriangles(solidQuadColor(pixel.R(0, 0, 1, 1), pixel.Alpha(0.5))).Draw()

	// premultiplied: 0.5*white{128,128,128} over 0.5*blue{0,0,128}
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 128, G: 128, B: 255, A: 255})
}

// ComposeCopy replaces the destination instead of blending.
func TestPictureData_ComposeCopy(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 1, 1))

	pd.MakeTriangles(solidQuadColor(pixel.R(0, 0, 1, 1), pixel.RGB(0, 0, 1))).Draw()

	pd.SetComposeMethod(pixel.ComposeCopy)
	pd.MakeTriangles(solidQuadColor(pixel.R(0, 0, 1, 1), pixel.RGBA{R: 0.25, G: 0.25, B: 0.25, A: 0.5})).Draw()

	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: 64, G: 64, B: 64, A: 128})
}

// Two semi-transparent triangles sharing an edge must not double-cover the shared pixel row.
func TestPictureData_SharedEdgeNotDoubleBlended(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 2))

	// two rectangles side by side, each 2 wide, meeting exactly at x = 2
	mkRect := func(minX, maxX float64) *pixel.TrianglesData {
		tri := pixel.MakeTrianglesData(6)
		pts := [6]pixel.Vec{
			{X: minX, Y: 0},
			{X: maxX, Y: 0},
			{X: maxX, Y: 2},
			{X: minX, Y: 0},
			{X: maxX, Y: 2},
			{X: minX, Y: 2},
		}
		for i, p := range pts {
			(*tri)[i].Position = p
			(*tri)[i].Color = pixel.Alpha(0.5)
		}
		return tri
	}

	pd.MakeTriangles(mkRect(0, 2)).Draw()
	pd.MakeTriangles(mkRect(2, 4)).Draw()

	// pixels of each rectangle are blended exactly once
	once := uint8(math.Round(0.5 * 255))
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{R: once, G: once, B: once, A: once})
	assertPixelRGBA(t, pd, 1, 0, color.RGBA{R: once, G: once, B: once, A: once})
	assertPixelRGBA(t, pd, 2, 0, color.RGBA{R: once, G: once, B: once, A: once})
	assertPixelRGBA(t, pd, 3, 0, color.RGBA{R: once, G: once, B: once, A: once})
}

// Two semi-transparent textured quads sharing an edge must not double-cover the shared pixel row.
func TestPictureData_SharedEdgeTexturedNotDoubleBlended(t *testing.T) {
	// 50% opaque red texture, 4 wide
	srcImg := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			srcImg.SetRGBA(x, y, color.RGBA{R: 128, G: 0, B: 0, A: 128})
		}
	}
	src := pixel.PictureDataFromImage(srcImg)
	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 2))

	mkTexturedRect := func(minX, maxX float64) *pixel.TrianglesData {
		tri := pixel.MakeTrianglesData(6)
		pts := [6]pixel.Vec{
			{X: minX, Y: 0},
			{X: maxX, Y: 0},
			{X: maxX, Y: 2},
			{X: minX, Y: 0},
			{X: maxX, Y: 2},
			{X: minX, Y: 2},
		}
		for i, p := range pts {
			(*tri)[i].Position = p
			(*tri)[i].Color = pixel.Alpha(1)
			(*tri)[i].Picture = p
			(*tri)[i].Intensity = 1
		}
		return tri
	}

	pd.MakePicture(src).Draw(pd.MakeTriangles(mkTexturedRect(0, 2)))
	pd.MakePicture(src).Draw(pd.MakeTriangles(mkTexturedRect(2, 4)))

	// every pixel is blended exactly once. color.RGBA stores premultiplied values, so a
	// 50%-alpha red texel {R:128, A:128} over transparent stays exactly {128, 0, 0, 128}
	// (double coverage would have pushed A and R noticeably higher).
	want := color.RGBA{R: 128, A: 128}
	for _, x := range []int{0, 1, 2, 3} {
		assertPixelRGBA(t, pd, x, 0, want)
		assertPixelRGBA(t, pd, x, 1, want)
	}
}

// A Batch accumulates sprites and draws them onto a PictureData in one flush.
func TestPictureData_Batch(t *testing.T) {
	src := solidPicture(2, color.RGBA{R: 255, A: 255})
	batch := pixel.NewBatch(&pixel.TrianglesData{}, src)

	sprite := pixel.NewSprite(src, src.Bounds())
	sprite.Draw(batch, pixel.IM.Moved(pixel.V(2, 2)))

	pd := pixel.MakePictureData(pixel.R(0, 0, 4, 4))
	batch.Draw(pd)

	assertPixelRGBA(t, pd, 1, 1, color.RGBA{R: 255, A: 255})
	assertPixelRGBA(t, pd, 2, 2, color.RGBA{R: 255, A: 255})
	assertPixelRGBA(t, pd, 0, 0, color.RGBA{})
	assertPixelRGBA(t, pd, 3, 3, color.RGBA{})
}

// Non-square targets with non-zero origins index pixels correctly.
func TestPictureData_NonZeroOrigin(t *testing.T) {
	pd := pixel.MakePictureData(pixel.R(10, 20, 12, 22))

	tri := pixel.MakeTrianglesData(3)
	(*tri)[0].Position = pixel.V(10, 20)
	(*tri)[1].Position = pixel.V(12, 20)
	(*tri)[2].Position = pixel.V(10, 22)
	for i := range *tri {
		(*tri)[i].Color = pixel.RGB(1, 0, 0)
	}
	pd.MakeTriangles(tri).Draw()

	assertPixelRGBA(t, pd, 10, 20, color.RGBA{R: 255, A: 255})
	// existing Index/Bounds semantics must still hold
	if got := pd.Bounds(); got != pixel.R(10, 20, 12, 22) {
		t.Fatalf("Bounds = %v, want Rect(10, 20, 12, 22)", got)
	}
	if got := pd.Index(pixel.V(11, 21)); got != 1*2+1 {
		t.Fatalf("Index(11,21) = %d, want %d", got, 3)
	}
}
