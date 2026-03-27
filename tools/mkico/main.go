// tools/mkico converts a PNG to a multi-size ICO file (PNG-encoded entries).
// Usage: go run ./tools/mkico <input.png> <output.ico>
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: mkico <input.png> <output.ico>")
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	sizes := []int{16, 32, 48, 256}
	var pngs [][]byte
	for _, sz := range sizes {
		resized := resize(src, sz, sz)
		tmp, err := os.CreateTemp("", "ico_*.png")
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(tmp, resized); err != nil {
			log.Fatal(err)
		}
		tmp.Close()
		data, err := os.ReadFile(tmp.Name())
		if err != nil {
			log.Fatal(err)
		}
		os.Remove(tmp.Name())
		pngs = append(pngs, data)
	}

	out, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	n := len(sizes)
	// ICONDIR header
	writeU16(out, 0) // reserved
	writeU16(out, 1) // type = icon
	writeU16(out, uint16(n))

	// ICONDIRENTRY array
	offset := uint32(6 + n*16)
	for i, sz := range sizes {
		w, h := uint8(sz), uint8(sz)
		if sz == 256 {
			w, h = 0, 0
		}
		out.Write([]byte{w, h, 0, 0})       // width, height, colorCount, reserved
		writeU16(out, 1)                     // planes
		writeU16(out, 32)                    // bitCount
		writeU32(out, uint32(len(pngs[i]))) // size
		writeU32(out, offset)               // offset
		offset += uint32(len(pngs[i]))
	}

	for _, data := range pngs {
		out.Write(data)
	}
}

func writeU16(f *os.File, v uint16) {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	f.Write(b)
}
func writeU32(f *os.File, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	f.Write(b)
}

// resize scales src to (dw x dh) using bilinear interpolation.
func resize(src image.Image, dw, dh int) image.Image {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			// map dst pixel to src coordinates
			fx := float64(x) * float64(sw-1) / float64(dw-1)
			fy := float64(y) * float64(sh-1) / float64(dh-1)
			x0 := int(fx)
			y0 := int(fy)
			x1 := x0 + 1
			y1 := y0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			if y1 >= sh {
				y1 = sh - 1
			}
			dx := fx - float64(x0)
			dy := fy - float64(y0)

			c00 := toNRGBA(src.At(sb.Min.X+x0, sb.Min.Y+y0))
			c10 := toNRGBA(src.At(sb.Min.X+x1, sb.Min.Y+y0))
			c01 := toNRGBA(src.At(sb.Min.X+x0, sb.Min.Y+y1))
			c11 := toNRGBA(src.At(sb.Min.X+x1, sb.Min.Y+y1))

			dst.SetNRGBA(x, y, color.NRGBA{
				R: lerp2(c00.R, c10.R, c01.R, c11.R, dx, dy),
				G: lerp2(c00.G, c10.G, c01.G, c11.G, dx, dy),
				B: lerp2(c00.B, c10.B, c01.B, c11.B, dx, dy),
				A: lerp2(c00.A, c10.A, c01.A, c11.A, dx, dy),
			})
		}
	}
	return dst
}

func toNRGBA(c color.Color) color.NRGBA {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: uint8(r * 0xff / a),
		G: uint8(g * 0xff / a),
		B: uint8(b * 0xff / a),
		A: uint8(a >> 8),
	}
}

func lerp2(c00, c10, c01, c11 uint8, dx, dy float64) uint8 {
	top := float64(c00)*(1-dx) + float64(c10)*dx
	bot := float64(c01)*(1-dx) + float64(c11)*dx
	return uint8(top*(1-dy) + bot*dy)
}
