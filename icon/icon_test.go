package icon

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func isAmber(c color.Color) bool {
	r, g, b, a := c.RGBA()
	return a>>8 == 255 && r>>8 == 245 && g>>8 == 158 && b>>8 == 11
}

// The badge has to land in the top-right corner and nowhere else, or it would
// cover the glyph or collide with the recording dot.
func TestWithWarningBadgeTopRight(t *testing.T) {
	for _, size := range []int{22, 64} {
		base := Scale(Data, size)
		img := decode(t, WithWarningBadge(base))
		b := img.Bounds()
		if b.Dx() != size || b.Dy() != size {
			t.Fatalf("size %d: got %dx%d", size, b.Dx(), b.Dy())
		}

		var found bool
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if !isAmber(img.At(x, y)) {
					continue
				}
				found = true
				if x < size/2 || y > size/2 {
					t.Errorf("size %d: amber pixel at (%d,%d), outside the top-right quadrant", size, x, y)
				}
			}
		}
		if !found {
			t.Errorf("size %d: no amber pixel, badge not drawn", size)
		}
	}
}

// Recording and a failed sync happen at once, so the badge must leave the dot
// in the opposite corner intact.
func TestWithWarningBadgeKeepsRecordingDot(t *testing.T) {
	base := Scale(Data, 64)
	dotted := decode(t, WithRecordingDot(base))
	both := decode(t, WithWarningBadge(WithRecordingDot(base)))

	// Centre of the dot, per WithRecordingDot's own geometry.
	radius := 64 / 5
	cx, cy := 64-radius-1, 64-radius-1
	if got, want := both.At(cx, cy), dotted.At(cx, cy); got != want {
		t.Errorf("dot centre changed: got %v, want %v", got, want)
	}
	// Well inside the triangle: horizontally centred on the badge, low enough
	// down that the shape has widened out.
	badge := 64 * 45 / 100
	if !isAmber(both.At(64-1-badge/2, badge)) {
		t.Errorf("badge missing from the top-right corner")
	}
}

// A caller that hands over something that is not a PNG gets its input back
// rather than a panic, matching WithRecordingDot and Recolor.
func TestWithWarningBadgeBadInput(t *testing.T) {
	in := []byte("not a png")
	if got := WithWarningBadge(in); !bytes.Equal(got, in) {
		t.Errorf("got %q, want the input back", got)
	}
}
