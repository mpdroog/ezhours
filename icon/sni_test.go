package icon

import (
	"image/color"
	"testing"
)

// systrayPixel reproduces fyne.io/systray's argbForImage for one pixel: the low
// byte of each RGBA() channel.
func systrayPixel(c color.Color) (a, r, g, b uint8) {
	r32, g32, b32, a32 := c.RGBA()
	return byte(a32), byte(r32), byte(g32), byte(b32)
}

// Every pixel, anti-aliased edges included, must reach the tray host as the
// straight colour and alpha that was drawn.
func TestForStatusNotifierSurvivesSystray(t *testing.T) {
	light := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	drawn := WithWarningBadge(WithRecordingDot(Glyph(64, light)))
	want := toNRGBA(decode(t, drawn))
	got := decode(t, ForStatusNotifier(drawn))

	b := want.Bounds()
	var partial int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			w := want.NRGBAAt(x, y)
			if w.A == 0 {
				continue
			}
			if w.A != 255 {
				partial++
			}
			a, r, g, bl := systrayPixel(got.At(x, y))
			if a != w.A || r != w.R || g != w.G || bl != w.B {
				t.Fatalf("(%d,%d): systray sends ARGB %d,%d,%d,%d, want %d,%d,%d,%d",
					x, y, a, r, g, bl, w.A, w.R, w.G, w.B)
			}
		}
	}
	if partial == 0 {
		t.Fatal("no partially transparent pixels: nothing anti-aliased, test proves nothing")
	}
}
