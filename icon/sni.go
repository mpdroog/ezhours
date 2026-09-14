package icon

import (
	"bytes"
	"image"
	"image/color"
)

// ForStatusNotifier re-encodes a finished tray PNG so that fyne.io/systray hands
// the StatusNotifier host the pixels it actually contains. Run it last, on Linux
// only, after every badge is drawn.
//
// systray builds the ARGB pixmap by truncating each color.Color RGBA() value
// with byte(), keeping the low byte of a 16-bit alpha-premultiplied channel
// rather than the high byte of a straight one. That is exact for pixels that are
// fully opaque or fully transparent and garbage for everything in between: every
// anti-aliased edge arrives dark or speckled. (Still so as of v1.12.2.)
//
// So store each pixel as 16-bit straight alpha, picking the channel values whose
// premultiplied form has the intended 8-bit straight value as its low byte.
func ForStatusNotifier(data []byte) []byte {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	n := toNRGBA(src)
	b := n.Bounds()
	dst := image.NewNRGBA64(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := n.NRGBAAt(x, y)
			a := uint32(c.A) * 0x101
			dst.SetNRGBA64(x, y, color.NRGBA64{
				R: sniChannel(c.R, a),
				G: sniChannel(c.G, a),
				B: sniChannel(c.B, a),
				A: uint16(a),
			})
		}
	}

	return encode(dst, data)
}

// sniChannel returns the smallest 16-bit channel value c for which
// color.NRGBA64.RGBA's c*a/0xffff has v as its low byte. Each step in c moves
// that product by a/0xffff, under one, so rounding up lands on v exactly.
func sniChannel(v uint8, a uint32) uint16 {
	if a == 0 || a == 0xffff {
		return uint16(v) * 0x101
	}
	return uint16((uint32(v)*0xffff + a - 1) / a)
}
