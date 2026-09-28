//go:build linux

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// A row clicked while the app is still busy with the last click is one
// click, not a queue of them: the second would arrive after the dialog it
// opens had closed.
func TestFireDoesNotBlockOrQueue(t *testing.T) {
	m := &MenuItem{clicked: make(chan struct{}, 1)}
	m.fire()
	m.fire() // must not block
	m.fire()
	if len(m.clicked) != 1 {
		t.Fatalf("queued %d clicks, want 1", len(m.clicked))
	}
	<-m.Clicked()
	select {
	case <-m.Clicked():
		t.Error("a second click was delivered")
	default:
	}
}

func TestSetTitleIsWhatTheMenuDraws(t *testing.T) {
	m := &MenuItem{title: "Start Timer", clicked: make(chan struct{}, 1)}
	if got := m.label(); got != "Start Timer" {
		t.Errorf("label() = %q", got)
	}
	m.SetTitle("Stop Timer (00:00:05)")
	if got := m.label(); got != "Stop Timer (00:00:05)" {
		t.Errorf("label() after SetTitle = %q", got)
	}
}

func testPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The tray picks whichever size is closest to what it draws, so every one
// has to be there and be the size it claims.
func TestPixmapsCoverEverySize(t *testing.T) {
	got, err := pixmaps(testPNG(t, 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(iconSizes) {
		t.Fatalf("got %d pixmaps, want %d", len(got), len(iconSizes))
	}
	for i, p := range got {
		want := int32(iconSizes[i])
		if p.Width != want || p.Height != want {
			t.Errorf("pixmap %d is %dx%d, want %dx%d", i, p.Width, p.Height, want, want)
		}
		if n := len(p.Pix); n != int(p.Width*p.Height*4) {
			t.Errorf("pixmap %d has %d bytes, want %d", i, n, p.Width*p.Height*4)
		}
	}
}

// A PNG that is not really a PNG must be reported, not drawn as noise.
func TestPixmapsRejectsRubbish(t *testing.T) {
	if _, err := pixmaps([]byte("not a png")); err == nil {
		t.Error("pixmaps() accepted rubbish")
	}
}

func TestScaleToKeepsColourAndSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			src.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	for _, size := range []int{8, 22, 64} {
		dst := scaleTo(src, size)
		if b := dst.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Fatalf("scaleTo(%d) gave %dx%d", size, b.Dx(), b.Dy())
		}
		if got := dst.RGBAAt(size/2, size/2); got != (color.RGBA{R: 10, G: 20, B: 30, A: 255}) {
			t.Errorf("scaleTo(%d) centre pixel = %v", size, got)
		}
	}
}
