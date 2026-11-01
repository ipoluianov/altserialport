// Package appicon draws the icon of the program at any size: for the
// window, and for the packages of Linux, Windows and macOS
package appicon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/ipoluianov/nui/ui"
)

// side is the size the icon is drawn in; other sizes are the same drawing
// at another scale of the canvas
const side = 256

var (
	blue  = color.RGBA{0x1f, 0x6f, 0xd6, 0xff}
	white = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// Image draws the icon of the program, size x size pixels: a plug of a
// serial port with a prompt in it. A size of 0 or less gives 256.
func Image(size int) image.Image {
	if size <= 0 {
		size = side
	}
	cnv, img := ui.NewImageCanvas(side, side, float64(size)/side)
	cnv.SetColor(blue)
	cnv.FillRoundedRect(8, 8, side-16, side-16, 48)
	// The trapezoid of a DE-9 connector
	body, _ := ui.ParseSVGPath("M44 78h168l-22 100H66z")
	cnv.FillPath(body, white)
	pins := ui.NewPath()
	for i := 0; i < 5; i++ {
		pins.Circle(76+float64(i)*26, 108, 8)
	}
	for i := 0; i < 4; i++ {
		pins.Circle(89+float64(i)*26, 146, 8)
	}
	cnv.FillPath(pins, blue)
	return img
}

// PNG encodes the icon of the size as a PNG file
func PNG(size int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, Image(size)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ICO writes an .ico file with a PNG image of each size, as Windows
// Vista and later read them: a header of 6 bytes, a directory entry of 16
// bytes for each image, then the PNG files one after another
func ICO(sizes []int) ([]byte, error) {
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		if size < 1 || size > 256 {
			return nil, fmt.Errorf("an .ico image cannot be %d pixels", size)
		}
		data, err := PNG(size)
		if err != nil {
			return nil, err
		}
		images[i] = data
	}
	var buf bytes.Buffer
	le := binary.LittleEndian
	w16 := func(v uint16) { buf.Write(le.AppendUint16(nil, v)) }
	w32 := func(v uint32) { buf.Write(le.AppendUint32(nil, v)) }
	w16(0) // reserved
	w16(1) // the type: an icon
	w16(uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, size := range sizes {
		// 256 is written as 0: the field is one byte
		dim := byte(size)
		if size == 256 {
			dim = 0
		}
		buf.WriteByte(dim) // width
		buf.WriteByte(dim) // height
		buf.WriteByte(0)   // colors in the palette: none
		buf.WriteByte(0)   // reserved
		w16(1)             // color planes
		w16(32)            // bits per pixel
		w32(uint32(len(images[i])))
		w32(uint32(offset))
		offset += len(images[i])
	}
	for _, data := range images {
		buf.Write(data)
	}
	return buf.Bytes(), nil
}
