package appicon

import "testing"

func TestImageSizes(t *testing.T) {
	for _, size := range []int{16, 32, 48, 256, 1024} {
		img := Image(size)
		b := img.Bounds()
		if b.Dx() != size || b.Dy() != size {
			t.Fatalf("size %d: got %dx%d", size, b.Dx(), b.Dy())
		}
		// The corner is transparent, the middle of the plug is white
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("size %d: the corner is not transparent", size)
		}
		r, g, bl, _ := img.At(size/2, size*90/256).RGBA()
		if r>>8 < 0xf0 || g>>8 < 0xf0 || bl>>8 < 0xf0 {
			t.Errorf("size %d: the plug is not white: %x %x %x", size, r>>8, g>>8, bl>>8)
		}
	}
}
