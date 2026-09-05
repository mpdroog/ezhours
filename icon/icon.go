package icon

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	_ "image/png"
	"math"
	"sync"
)

//go:embed clock.png
var Data []byte

var (
	activeOnce sync.Once
	activeData []byte
)

// Scale returns the given PNG icon data resized to size x size pixels using bilinear interpolation.
func Scale(data []byte, size int) []byte {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())

	clamp := func(v, max int) int {
		if v < 0 {
			return 0
		}
		if v >= max {
			return max - 1
		}
		return v
	}
	lerp := func(a, b, t float64) float64 { return a*(1-t) + b*t }
	sample := func(px, py int) (float64, float64, float64, float64) {
		r, g, b, a := src.At(clamp(px, sb.Dx()), clamp(py, sb.Dy())).RGBA()
		return float64(r) / 65535, float64(g) / 65535, float64(b) / 65535, float64(a) / 65535
	}

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := (float64(x)+0.5)/float64(size)*sw - 0.5
			sy := (float64(y)+0.5)/float64(size)*sh - 0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-math.Floor(sx), sy-math.Floor(sy)

			r00, g00, b00, a00 := sample(x0, y0)
			r10, g10, b10, a10 := sample(x0+1, y0)
			r01, g01, b01, a01 := sample(x0, y0+1)
			r11, g11, b11, a11 := sample(x0+1, y0+1)

			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(lerp(lerp(r00, r10, fx), lerp(r01, r11, fx), fy) * 255),
				G: uint8(lerp(lerp(g00, g10, fx), lerp(g01, g11, fx), fy) * 255),
				B: uint8(lerp(lerp(b00, b10, fx), lerp(b01, b11, fx), fy) * 255),
				A: uint8(lerp(lerp(a00, a10, fx), lerp(a01, a11, fx), fy) * 255),
			})
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, dst)
	return buf.Bytes()
}

// ActiveData returns the clock icon with a red recording dot in the bottom-right
// corner. The result is computed once and cached.
func ActiveData() []byte {
	activeOnce.Do(func() { activeData = WithRecordingDot(Data) })
	return activeData
}

// WithRecordingDot draws the red recording dot in the bottom-right corner of the
// given PNG. Call it after Scale and Recolor: the dot is sized to the image it is
// drawn on, and drawing it last keeps Recolor from painting over it.
func WithRecordingDot(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	bounds := img.Bounds()
	active := image.NewRGBA(bounds)
	draw.Draw(active, bounds, img, bounds.Min, draw.Src)

	w, h := bounds.Dx(), bounds.Dy()
	radius := w / 5
	if radius < 2 {
		radius = 2
	}
	cx, cy := w-radius-1, h-radius-1

	red := color.RGBA{R: 220, G: 50, B: 50, A: 255}
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if x >= 0 && y >= 0 && x < w && y < h {
				dx, dy := float64(x-cx), float64(y-cy)
				if math.Sqrt(dx*dx+dy*dy) <= float64(radius) {
					active.SetRGBA(x, y, red)
				}
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, active)
	return buf.Bytes()
}

// Recolor repaints every pixel of the given PNG in c, keeping the alpha channel
// as it is. clock.png is a macOS template icon -- a black glyph on transparent --
// which macOS inverts for a dark menu bar but Linux tray hosts draw as-is, black
// on a black panel. Recolor is how the Linux side picks a glyph colour that the
// panel it lands on can actually show.
//
// Run it on the already scaled image: Scale interpolates alpha-premultiplied
// samples, which is exact for a black glyph but would fringe a white one grey.
func Recolor(data []byte, c color.RGBA) []byte {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			dst.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: uint8(a >> 8)})
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, dst)
	return buf.Bytes()
}

// WithWarningBadge draws an amber warning triangle in the top-right corner of
// the given PNG, marking a sync that failed. Like WithRecordingDot it is drawn
// last, after Scale and Recolor, so the badge keeps its colour; unlike the dot
// it sits in the opposite corner, so a recording timer and a failed sync stay
// legible at the same time.
func WithWarningBadge(data []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	bounds := img.Bounds()
	badged := image.NewRGBA(bounds)
	draw.Draw(badged, bounds, img, bounds.Min, draw.Src)

	w, h := bounds.Dx(), bounds.Dy()
	size := w * 45 / 100
	if size < 5 {
		size = 5
	}
	// Top-right corner, one pixel clear of the edge.
	left, top := w-size-1, 1
	cx := float64(left) + float64(size)/2

	amber := color.RGBA{R: 245, G: 158, B: 11, A: 255}
	for y := top; y <= top+size && y < h; y++ {
		// The triangle widens from a point at the top to a full base.
		half := float64(y-top) / float64(size) * float64(size) / 2
		for x := int(cx - half); x <= int(cx+half); x++ {
			if x >= 0 && x < w && y >= 0 {
				badged.SetRGBA(x, y, amber)
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, badged)
	return buf.Bytes()
}
