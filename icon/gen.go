//go:build ignore

// Command gen draws the Mote app icon: a warm paper squircle holding the note
// glyph that the menu bar icon also uses, its last line in the app's amber.
//
//	go run icon/gen.go -o resources/icon.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const size = 1024

func main() {
	out := flag.String("o", "resources/icon.png", "where to write the PNG")
	flag.Parse()

	img := render()
	if err := os.WriteFile(*out, encode(img), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write icon:", err)
		os.Exit(1)
	}
}

func encode(img *image.NRGBA) []byte {
	var buf byteBuf
	_ = png.Encode(&buf, img)
	return buf.b
}

type byteBuf struct{ b []byte }

func (w *byteBuf) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// squircle is the rounded square macOS apps sit in: a superellipse with a
// continuous-feeling corner, inset a margin from the icon's edge.
type squircle struct {
	cx, cy, half float64
	exponent     float64
}

func (s squircle) contains(x, y float64) bool {
	dx := math.Abs(x-s.cx) / s.half
	dy := math.Abs(y-s.cy) / s.half
	return math.Pow(dx, s.exponent)+math.Pow(dy, s.exponent) <= 1
}

// roundedRect is one bar of the note glyph.
type roundedRect struct {
	x0, y0, x1, y1, radius float64
	amber                  bool
}

func (r roundedRect) contains(x, y float64) bool {
	cx, cy := (r.x0+r.x1)/2, (r.y0+r.y1)/2
	hw, hh := (r.x1-r.x0)/2, (r.y1-r.y0)/2
	dx := math.Abs(x-cx) - (hw - r.radius)
	dy := math.Abs(y-cy) - (hh - r.radius)
	if dx <= 0 && dy <= 0 {
		return true
	}
	dx = math.Max(dx, 0)
	dy = math.Max(dy, 0)
	return dx*dx+dy*dy <= r.radius*r.radius
}

func render() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// macOS places an icon's body inside a margin; 824 of 1024 matches the
	// system grid closely enough to sit naturally beside other apps.
	const margin = 100.0
	body := squircle{
		cx:       size / 2,
		cy:       size / 2,
		half:     (size - 2*margin) / 2,
		exponent: 4.6,
	}

	// The note sits centered in the squircle; its frame is squared so it reads
	// as a sheet of paper rather than a badge.
	const (
		paperW = 352.0
		paperH = 452.0
		line   = 24.0
	)
	left := (size - paperW) / 2
	top := (size - paperH) / 2
	right := left + paperW
	bottom := top + paperH

	sheet := roundedRect{left + line, top + line, right - line, bottom - line, 6, false}
	sheetColor := color.NRGBA{R: 0xFF, G: 0xFD, B: 0xF7, A: 0xFF}

	const (
		text  = 26.0 // height of a line of writing
		inset = 88.0 // room between the frame and the writing
		gap   = 84.0 // between lines
	)
	bars := []roundedRect{
		{left, top, right, top + line, 10, false},       // top of the frame
		{left, bottom - line, right, bottom, 10, false}, // bottom of the frame
		{left, top, left + line, bottom, 10, false},     // left of the frame
		{right - line, top, right, bottom, 10, false},   // right of the frame
		{left + inset, top + 96, right - inset, top + 96 + text, 13, false},
		{left + inset, top + 96 + gap, right - inset, top + 96 + gap + text, 13, false},
		{left + inset, top + 96 + 2*gap, right - inset - 70, top + 96 + 2*gap + text, 13, true},
	}

	paperTop := color.NRGBA{R: 0xFF, G: 0xFC, B: 0xF3, A: 0xFF}
	paperBottom := color.NRGBA{R: 0xF1, G: 0xE3, B: 0xC6, A: 0xFF}
	ink := color.NRGBA{R: 0x2B, G: 0x27, B: 0x22, A: 0xFF}
	amber := color.NRGBA{R: 0xE3, G: 0xA9, B: 0x3E, A: 0xFF}
	rim := color.NRGBA{R: 0x8A, G: 0x76, B: 0x50, A: 0x22}

	// 4x4 supersampling gives clean edges without a resampler.
	const samples = 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var bodyCover, sheetCover, inkCover, amberCover, rimCover float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/samples
					py := float64(y) + (float64(sy)+0.5)/samples
					if !body.contains(px, py) {
						continue
					}
					bodyCover++
					if n := body.edgeDistance(px, py); n < 3 {
						rimCover++
					}
					if sheet.contains(px, py) {
						sheetCover++
					}
					// The bars overlap at the frame's corners: count their
					// union, or the overlap would blend the ink twice over.
					inkHere, amberHere := false, false
					for _, bar := range bars {
						if !bar.contains(px, py) {
							continue
						}
						if bar.amber {
							amberHere = true
						} else {
							inkHere = true
						}
					}
					if inkHere {
						inkCover++
					} else if amberHere {
						amberCover++
					}
				}
			}
			total := float64(samples * samples)
			if bodyCover == 0 {
				continue
			}

			t := float64(y) / float64(size-1)
			base := mix(paperTop, paperBottom, t)
			base = blend(base, rim, rimCover/total)
			base = blend(base, sheetColor, sheetCover/total)
			base = blend(base, ink, inkCover/total)
			base = blend(base, amber, amberCover/total)

			a := bodyCover / total
			img.SetNRGBA(x, y, color.NRGBA{
				R: base.R,
				G: base.G,
				B: base.B,
				A: uint8(math.Round(255 * a)),
			})
		}
	}
	return img
}

// edgeDistance approximates how far a point is inside body's rim, in pixels,
// by sampling inward.
func (s squircle) edgeDistance(x, y float64) float64 {
	for d := 1.0; d < 8; d += 1 {
		if !s.contains(x+d, y) {
			return d
		}
		if !s.contains(x-d, y) {
			return d
		}
		if !s.contains(x, y+d) {
			return d
		}
		if !s.contains(x, y-d) {
			return d
		}
	}
	return 8
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(math.Round(float64(a.R) + (float64(b.R)-float64(a.R))*t)),
		G: uint8(math.Round(float64(a.G) + (float64(b.G)-float64(a.G))*t)),
		B: uint8(math.Round(float64(a.B) + (float64(b.B)-float64(a.B))*t)),
		A: 0xFF,
	}
}

func blend(dst, src color.NRGBA, alpha float64) color.NRGBA {
	if alpha <= 0 {
		return dst
	}
	a := alpha * float64(src.A) / 255
	return color.NRGBA{
		R: uint8(math.Round(float64(dst.R)*(1-a) + float64(src.R)*a)),
		G: uint8(math.Round(float64(dst.G)*(1-a) + float64(src.G)*a)),
		B: uint8(math.Round(float64(dst.B)*(1-a) + float64(src.B)*a)),
		A: 0xFF,
	}
}
