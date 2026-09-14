package icon

import (
	"image"
	"image/color"
	"math"
)

// subsamples is the per-axis grid paint tests inside each pixel; 4 gives 17
// coverage levels, plenty for edges a tray host will scale again anyway.
const subsamples = 4

// Glyph draws the clock at size x size pixels in c, anti-aliased, as a PNG.
//
// clock.png is 22 pixels: fine for a macOS menu bar, which picks its @2x itself,
// but a StatusNotifier host on a scaled desktop wants 40-60 device pixels, and
// stretching the bitmap that far turns its outline into a blurred staircase.
// Drawing the same shape from geometry keeps the edges clean at any size.
func Glyph(size int, c color.RGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	half := math.Max(s*0.09, 1.5) / 2
	cx, cy := s/2, s/2
	r := s/2 - half - s*0.04

	paint(img, color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}, func(x, y float64) bool {
		return math.Abs(math.Hypot(x-cx, y-cy)-r) <= half ||
			segmentDist(x, y, cx, cy, cx-r*0.42, cy-r*0.36) <= half ||
			segmentDist(x, y, cx, cy, cx+r*0.62, cy-r*0.12) <= half
	})
	return encode(img, Data)
}

// paint composites c over img, weighting each pixel by how much of a
// subsamples x subsamples grid inside it falls within the shape.
func paint(img *image.NRGBA, c color.NRGBA, inside func(x, y float64) bool) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			n := 0
			for sy := 0; sy < subsamples; sy++ {
				for sx := 0; sx < subsamples; sx++ {
					if inside(float64(x)+(float64(sx)+0.5)/subsamples, float64(y)+(float64(sy)+0.5)/subsamples) {
						n++
					}
				}
			}
			if n > 0 {
				over(img, x, y, c, float64(n)/(subsamples*subsamples))
			}
		}
	}
}

// over blends c, at the given coverage, onto the pixel at (x, y) with the
// Porter-Duff over operator in straight (non-premultiplied) alpha.
func over(img *image.NRGBA, x, y int, c color.NRGBA, coverage float64) {
	d := img.NRGBAAt(x, y)
	sa := float64(c.A) / 255 * coverage
	da := float64(d.A) / 255 * (1 - sa)
	oa := sa + da
	if oa == 0 {
		return
	}
	mix := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*sa + float64(d)*da) / oa))
	}
	img.SetNRGBA(x, y, color.NRGBA{
		R: mix(c.R, d.R),
		G: mix(c.G, d.G),
		B: mix(c.B, d.B),
		A: uint8(math.Round(oa * 255)),
	})
}

// segmentDist is the distance from (px, py) to the segment (ax, ay)-(bx, by).
func segmentDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}
