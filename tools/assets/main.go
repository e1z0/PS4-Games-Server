// SPDX-License-Identifier: GPL-2.0-only
// Generates the embedded raster fallback using Go alone; no external image tools.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	im := image.NewRGBA(image.Rect(0, 0, 400, 400))
	draw.Draw(im, im.Bounds(), &image.Uniform{color.RGBA{32, 43, 60, 255}}, image.Point{}, draw.Src)
	accent := color.RGBA{119, 216, 231, 255}
	for y := 88; y < 313; y++ {
		for x := 88; x < 313; x++ {
			d := abs(x-200) + abs(y-200)
			if d >= 108 && d <= 113 {
				im.Set(x, y, accent)
			}
		}
	}
	glyphs := [][]string{{"11110", "10001", "10001", "11110", "10000", "10000", "10000"}, {"01111", "10000", "10000", "01110", "00001", "00001", "11110"}, {"10010", "10010", "10010", "11111", "00010", "00010", "00010"}}
	for i, g := range glyphs {
		for y, row := range g {
			for x, p := range row {
				if p == '1' {
					r := image.Rect(133+i*49+x*7, 176+y*7, 140+i*49+x*7, 183+y*7)
					draw.Draw(im, r, &image.Uniform{color.RGBA{237, 243, 252, 255}}, image.Point{}, draw.Src)
				}
			}
		}
	}
	f, e := os.Create("internal/server/ui/fallback.png")
	if e != nil {
		panic(e)
	}
	if e = png.Encode(f, im); e != nil {
		panic(e)
	}
	if e = f.Close(); e != nil {
		panic(e)
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
