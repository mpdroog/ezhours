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

// ActiveData returns the clock icon with a red recording dot in the bottom-right corner.
// The result is computed once and cached.
func ActiveData() []byte {
	activeOnce.Do(func() {
		img, _, err := image.Decode(bytes.NewReader(Data))
		if err != nil {
			activeData = Data
			return
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
		activeData = buf.Bytes()
	})
	return activeData
}
