package pixel

import (
	"image/color"
	"math"
)

// This file turns PictureData into an off-screen, OpenGL-free draw Target.
//
// The rasterization performed here mirrors the OpenGL pipeline implemented in the pixelgl package
// (see pixelgl/glshader.go):
//
//   - positions are projected by the Target's matrix
//   - per-vertex colors, picture coordinates and intensity are interpolated
//   - the picture is point-sampled at pixel centers
//   - a color mask is multiplied onto every fragment
//   - non-zero ClipRects discard fragments outside of them
//   - fragments are composited onto the destination with ComposeMethod.Compose, using
//     alpha-premultiplied RGBA throughout
//
// Triangles are rasterized with a top-left fill rule so that the shared edge of two adjacent
// triangles is covered exactly once.

var (
	_ Target        = (*PictureData)(nil)
	_ BasicTarget   = (*PictureData)(nil)
	_ ComposeTarget = (*PictureData)(nil)
)

// SetMatrix sets a Matrix that every point drawn onto this PictureData will be projected by.
func (pd *PictureData) SetMatrix(m Matrix) {
	pd.mat = m
}

// SetColorMask sets a color that every color in triangles or a picture drawn onto this
// PictureData will be multiplied by.
//
// A nil color means a fully opaque white mask (no effect).
func (pd *PictureData) SetColorMask(c color.Color) {
	if c == nil {
		pd.col = Alpha(1)
		return
	}
	pd.col = ToRGBA(c)
}

// SetComposeMethod sets a Porter-Duff composition method to be used in the following draws onto
// this PictureData.
func (pd *PictureData) SetComposeMethod(cmp ComposeMethod) {
	pd.cmp = cmp
}

// MakeTriangles creates a specialized copy of the provided Triangles that draws onto this
// PictureData.
//
// TrianglesPosition, TrianglesColor, TrianglesPicture and TrianglesClipped are supported.
func (pd *PictureData) MakeTriangles(t Triangles) TargetTriangles {
	return &pictureTriangles{
		tri: t.Copy(),
		dst: pd,
	}
}

// MakePicture creates a specialized copy of the provided Picture that draws onto this
// PictureData.
//
// PictureColor is supported.
func (pd *PictureData) MakePicture(p Picture) TargetPicture {
	return &pictureTargetPicture{
		pic: p,
		dst: pd,
	}
}

type pictureTriangles struct {
	tri Triangles
	pic Picture
	dst *PictureData
}

func (pt *pictureTriangles) Len() int {
	return pt.tri.Len()
}

func (pt *pictureTriangles) SetLen(length int) {
	pt.tri.SetLen(length)
}

func (pt *pictureTriangles) Slice(i, j int) Triangles {
	return &pictureTriangles{
		tri: pt.tri.Slice(i, j),
		pic: pt.pic,
		dst: pt.dst,
	}
}

func (pt *pictureTriangles) Update(t Triangles) {
	pt.tri.Update(t)
}

func (pt *pictureTriangles) Copy() Triangles {
	return &pictureTriangles{
		tri: pt.tri.Copy(),
		pic: pt.pic,
		dst: pt.dst,
	}
}

func (pt *pictureTriangles) Draw() {
	pt.dst.rasterize(pt.tri, pt.pic)
}

type pictureTargetPicture struct {
	pic Picture
	dst *PictureData
}

func (pp *pictureTargetPicture) Bounds() Rect {
	return pp.pic.Bounds()
}

func (pp *pictureTargetPicture) Draw(t TargetTriangles) {
	pt := t.(*pictureTriangles)
	if pp.dst != pt.dst {
		panic("(PictureData).MakePicture: TargetPicture drawn with TargetTriangles of a different Target")
	}
	pt.pic = pp.pic
	pt.Draw()
}

// rasterVertex holds the per-vertex data the rasterizer needs, in Target (post-matrix) position
// space for the position and in the original attribute space for everything else.
type rasterVertex struct {
	pos       Vec
	col       RGBA
	pic       Vec
	intensity float64
	clip      Rect
}

// rasterize draws all triangles contained in t onto the PictureData, optionally textured by pic.
func (pd *PictureData) rasterize(t Triangles, pic Picture) {
	n := t.Len()
	if n < 3 {
		return
	}

	positions, ok := t.(TrianglesPosition)
	if !ok {
		return
	}
	colors, _ := t.(TrianglesColor)
	pictures, _ := t.(TrianglesPicture)
	clips, _ := t.(TrianglesClipped)

	verts := make([]rasterVertex, n)
	for i := 0; i < n; i++ {
		v := rasterVertex{
			pos:       pd.mat.Project(positions.Position(i)),
			col:       Alpha(1),
			intensity: 0,
		}
		if colors != nil {
			v.col = colors.Color(i)
		}
		if pictures != nil {
			v.pic, v.intensity = pictures.Picture(i)
		}
		if clips != nil {
			v.clip, _ = clips.ClipRect(i)
		}
		verts[i] = v
	}

	for first := 0; first+2 < n; first += 3 {
		pd.rasterizeTriangle(verts[first], verts[first+1], verts[first+2], pic)
	}
}

// rasterizeTriangle rasterizes a single triangle using barycentric interpolation and a top-left
// fill rule (matching the OpenGL convention used by pixelgl).
func (pd *PictureData) rasterizeTriangle(a, b, c rasterVertex, pic Picture) {
	// Require a counter-clockwise winding so that the signed area below is positive; this keeps
	// the top-left fill rule signs uniform.
	if cross(b.pos.Sub(a.pos), c.pos.Sub(a.pos)) < 0 {
		b, c = c, b
	}

	area := cross(b.pos.Sub(a.pos), c.pos.Sub(a.pos))
	if area == 0 {
		return
	}
	invArea := 1 / area

	// Bounding box of the triangle, intersected with the pixel grid of this PictureData.
	minX := math.Min(math.Min(a.pos.X, b.pos.X), c.pos.X)
	maxX := math.Max(math.Max(a.pos.X, b.pos.X), c.pos.X)
	minY := math.Min(math.Min(a.pos.Y, b.pos.Y), c.pos.Y)
	maxY := math.Max(math.Max(a.pos.Y, b.pos.Y), c.pos.Y)

	bounds := pd.Rect
	x0 := int(math.Max(math.Floor(minX), math.Floor(bounds.Min.X)))
	y0 := int(math.Max(math.Floor(minY), math.Floor(bounds.Min.Y)))
	x1 := int(math.Min(math.Ceil(maxX), math.Ceil(bounds.Max.X)))
	y1 := int(math.Min(math.Ceil(maxY), math.Ceil(bounds.Max.Y)))

	// Edge vectors.
	edgeA := c.pos.Sub(b.pos) // edge opposite a: b -> c
	edgeB := a.pos.Sub(c.pos) // edge opposite b: c -> a
	edgeC := b.pos.Sub(a.pos) // edge opposite c: a -> b

	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			// Sample at the pixel center.
			p := V(float64(x)+0.5, float64(y)+0.5)

			// Barycentric coordinates (unnormalized): weights of a, b, c.
			wA := cross(edgeA, p.Sub(b.pos))
			wB := cross(edgeB, p.Sub(c.pos))
			wC := cross(edgeC, p.Sub(a.pos))

			// Top-left fill rule: a pixel is inside when all weights are positive, except
			// that a zero weight counts as inside exactly on a top or left edge.
			if wA < 0 || wB < 0 || wC < 0 {
				continue
			}
			if (wA == 0 && !isTopLeftEdge(edgeA)) ||
				(wB == 0 && !isTopLeftEdge(edgeB)) ||
				(wC == 0 && !isTopLeftEdge(edgeC)) {
				continue
			}

			ba := wA * invArea
			bb := wB * invArea
			bc := wC * invArea

			col := interpolateColor(a.col, b.col, c.col, ba, bb, bc)
			tex := interpolateVec(a.pic, b.pic, c.pic, ba, bb, bc)
			intensity := a.intensity*ba + b.intensity*bb + c.intensity*bc
			clip := interpolateClip(a.clip, b.clip, c.clip, ba, bb, bc)

			// ClipRect discard, equivalent to the GLSL discard in pixelgl's fragment shader,
			// which clips whenever the (interpolated) clip rect is not the zero rect.
			if clip != ZR {
				if p.X < clip.Min.X || p.Y < clip.Min.Y || p.X > clip.Max.X || p.Y > clip.Max.Y {
					continue
				}
			}

			frag := pd.fragment(col, tex, intensity, pic)
			pd.compositePixel(x, y, frag)
		}
	}
}

// isTopLeftEdge reports whether an oriented edge (for a counter-clockwise triangle) is a top or
// left edge. A left edge points upwards (dy < 0), a top edge is horizontal (dy == 0) and points
// rightwards (dx > 0).
func isTopLeftEdge(edge Vec) bool {
	return edge.Y < 0 || (edge.Y == 0 && edge.X > 0)
}

// cross returns the z-component of the cross product of a and b (a.X*b.Y - b.X*a.Y).
func cross(a, b Vec) float64 {
	return a.Cross(b)
}

func interpolateVec(a, b, c Vec, wa, wb, wc float64) Vec {
	return Vec{
		X: a.X*wa + b.X*wb + c.X*wc,
		Y: a.Y*wa + b.Y*wb + c.Y*wc,
	}
}

func interpolateColor(a, b, c RGBA, wa, wb, wc float64) RGBA {
	return RGBA{
		R: a.R*wa + b.R*wb + c.R*wc,
		G: a.G*wa + b.G*wb + c.G*wc,
		B: a.B*wa + b.B*wb + c.B*wc,
		A: a.A*wa + b.A*wb + c.A*wc,
	}
}

func interpolateClip(a, b, c Rect, wa, wb, wc float64) Rect {
	return Rect{
		Min: interpolateVec(a.Min, b.Min, c.Min, wa, wb, wc),
		Max: interpolateVec(a.Max, b.Max, c.Max, wa, wb, wc),
	}
}

// fragment computes the premultiplied RGBA color of one fragment, following the pixelgl canvas
// fragment shader:
//
//	frag = (1-intensity)*vColor + intensity*vColor*texture(texCoords)
//	frag *= colorMask
//
// When intensity is zero the picture is never sampled.
func (pd *PictureData) fragment(vColor RGBA, tex Vec, intensity float64, pic Picture) RGBA {
	var frag RGBA
	if intensity == 0 || pic == nil {
		frag = vColor
	} else {
		texColor := samplePicture(pic, tex)
		frag = vColor.Mul(Alpha(1 - intensity)).Add(vColor.Mul(texColor).Scaled(intensity))
	}
	return frag.Mul(pd.col)
}

// samplePicture point-samples a Picture at the given picture coordinate. Positions outside of the
// picture's bounds yield transparent black.
func samplePicture(pic Picture, at Vec) RGBA {
	bounds := pic.Bounds()
	if !bounds.Contains(at) {
		return RGBA{}
	}
	if pc, ok := pic.(PictureColor); ok {
		return pc.Color(at)
	}
	// Non-color pictures carry no per-pixel data; treat them as opaque white so that intensity
	// blending degrades to the vertex color.
	return Alpha(1)
}

// compositePixel blends one fragment into the destination pixel using the configured compose
// method. The pixel storage is 8-bit premultiplied RGBA.
func (pd *PictureData) compositePixel(x, y int, frag RGBA) {
	idx := pd.pixelIndex(x, y)
	if idx < 0 {
		return
	}
	dst := ToRGBA(pd.Pix[idx])
	out := pd.cmp.Compose(frag, dst)
	pd.Pix[idx] = color.RGBA{
		R: quantizeChannel(out.R),
		G: quantizeChannel(out.G),
		B: quantizeChannel(out.B),
		A: quantizeChannel(out.A),
	}
}

// pixelIndex returns the Pix index of the pixel at integer position (x, y), or -1 when outside of
// the PictureData's pixel grid.
func (pd *PictureData) pixelIndex(x, y int) int {
	minX := int(math.Floor(pd.Rect.Min.X))
	minY := int(math.Floor(pd.Rect.Min.Y))
	ix := x - minX
	iy := y - minY
	if ix < 0 || iy < 0 || ix >= pd.Stride || iy >= len(pd.Pix)/pd.Stride {
		return -1
	}
	return iy*pd.Stride + ix
}

// quantizeChannel converts a premultiplied color component in the [0, 1] range to an 8-bit value,
// rounding to nearest and clamping out-of-range results.
func quantizeChannel(v float64) uint8 {
	v = math.Round(v * 255)
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
