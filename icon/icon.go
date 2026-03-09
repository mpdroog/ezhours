package icon

import (
	"bytes"
	"image/png"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"math"
	"sync"
	_ "embed"
)

//go:embed clock.png
var Data []byte

var (
	activeOnce sync.Once
	activeData []byte
)

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
