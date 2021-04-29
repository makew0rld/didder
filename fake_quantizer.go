package main

import (
	"image"
	"image/color"
)

// Like in the dither library, see
// https://github.com/makeworld-the-better-one/dither/blob/3714c39500bc23a87a4fa14053344f201cc5beff/draw.go#L128-L156
// Use for specifying the recolor palette for GIF encoding

// fakeQuantizer implements draw.Quantizer. It ignores the provided image
// and just returns the provided palette each time. This is useful for places that
// only allow you to set the palette through a draw.Quantizer, like the image/gif
// package.
type fakeQuantizer struct {
	p []color.Color
}

func (fq *fakeQuantizer) Quantize(p color.Palette, m image.Image) color.Palette {
	return fq.p
}
