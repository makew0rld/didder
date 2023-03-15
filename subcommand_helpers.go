package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/makeworld-the-better-one/dither/v2"
	"github.com/mccutchen/palettor"
	"github.com/urfave/cli/v2"
	"golang.org/x/image/colornames"
)

// parsePercentArg takes a string like "0.5" or "50%" and will return a float
// like 50 or 0.5, depending on the second argument. An empty string returns 0.
//
// If `maxOne` is true, then "50%" will return 0.5. Otherwise it will return 50.
func parsePercentArg(arg string, maxOne bool) (float64, error) {
	if arg == "" {
		return 0, nil
	}
	if strings.HasSuffix(arg, "%") {
		arg = arg[:len(arg)-1]
		f64, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return 0, err
		}
		if maxOne {
			f64 /= 100.0
		}
		return f64, nil
	}
	f64, err := strconv.ParseFloat(arg, 64)
	if !maxOne {
		f64 *= 100.0
	}
	return f64, err
}

// globalFlag returns the value of flag at the top level of the command.
// For example, with the command:
//
//	dither --threads 1 edm -s Simple2D
//
// "threads" is a global flag, and "s" is a flag local to the edm subcommand.
func globalFlag(flag string, c *cli.Context) interface{} {
	ancestor := c.Lineage()[len(c.Lineage())-1]
	if len(ancestor.Args().Slice()) == 0 {
		// When the global context calls this func, the last in the lineage
		// has no args for some reason. So return the second-last instead.
		return c.Lineage()[len(c.Lineage())-2].Value(flag)
	}
	return ancestor.Value(flag)
}

// globalIsSet returns a bool indicating whether the provided global flag
// was actually set.
func globalIsSet(flag string, c *cli.Context) bool {
	ancestor := c.Lineage()[len(c.Lineage())-1]
	if len(ancestor.Args().Slice()) == 0 {
		// See globalFlag for why this if statement exists
		return c.Lineage()[len(c.Lineage())-2].IsSet(flag)
	}
	return ancestor.IsSet(flag)
}

// parseArgs takes arguments and splits them using the provided split characters.
func parseArgs(args []string, splitRunes string) []string {
	finalArgs := make([]string, 0)
	for _, arg := range args {
		finalArgs = append(finalArgs, strings.FieldsFunc(arg, func(c rune) bool {
			for _, c2 := range splitRunes {
				if c == c2 {
					return true
				}
			}
			return false
		})...)
	}
	return finalArgs
}

func hexToColor(hex string) (color.NRGBA, error) {
	// Modified from https://github.com/lucasb-eyer/go-colorful/blob/v1.2.0/colors.go#L333

	hex = strings.TrimPrefix(hex, "#")

	format := "%02x%02x%02x"
	var r, g, b uint8
	n, err := fmt.Sscanf(strings.ToLower(hex), format, &r, &g, &b)
	if err != nil {
		return color.NRGBA{}, err
	}
	if n != 3 {
		return color.NRGBA{}, fmt.Errorf("%s is not a hex color", hex)
	}
	return color.NRGBA{r, g, b, 255}, nil
}

func rgbToColor(s string) (color.NRGBA, error) {
	format := "%d,%d,%d"
	var r, g, b uint8
	n, err := fmt.Sscanf(s, format, &r, &g, &b)
	if err != nil {
		return color.NRGBA{}, err
	}
	if n != 3 {
		return color.NRGBA{}, fmt.Errorf("%s is not an RGB tuple", s)
	}
	return color.NRGBA{r, g, b, 255}, nil
}

func rgbaToColor(s string) (color.NRGBA, error) {
	format := "%d,%d,%d,%d"
	var r, g, b, a uint8
	n, err := fmt.Sscanf(s, format, &r, &g, &b, &a)
	if err != nil {
		return color.NRGBA{}, err
	}
	if n != 4 {
		return color.NRGBA{}, fmt.Errorf("%s is not an RGBA tuple", s)
	}
	// Parse as non-premult, as that's more user-friendly
	return color.NRGBA{r, g, b, a}, nil
}

// extractInputPalette extracts a 5-color palette from the first input image
// using palettor.
func extractInputPalette(flag string, c *cli.Context) ([]color.Color, error) {
	img, err := getInputImage(inputImages[0], c)
	if err != nil {
		return nil, fmt.Errorf("error loading image for palette extraction '%v': %w", inputImages, err)
	}

	// Resize: keep palettor.Extract fast. See the palettor CLI source:
	// https://github.com/mccutchen/palettor/blob/3eaed180/cmd/palettor/palettor.go#L57
	thumbnail := imaging.Resize(img, 200, 200, imaging.NearestNeighbor)

	// TODO: make these settings configurable, particularly the number of colors
	// in the palette. That means threading the argument through the CLI.
	palette, err := palettor.Extract(5, 500, thumbnail)
	if err != nil {
		return nil, fmt.Errorf("error extracting image palette: %w", err)
	}

	log.Printf("Extracted palette: %v", palette.Colors())
	return palette.Colors(), nil
}

// parseColors takes args and turns them into a color slice. All returned
// colors are guaranteed to only be color.NRGBA.
func parseColors(flag string, c *cli.Context) ([]color.Color, error) {
	args := parseArgs([]string{globalFlag(flag, c).(string)}, " ")

	if len(args) == 1 && args[0] == "sample" {
		return extractInputPalette(flag, c)
	}

	colors := make([]color.Color, len(args))

	for i, arg := range args {
		// Try to parse as RGB numbers, then hex, then grayscale, then SVG colors, then fail
		// Optionally try for RGBA if it's recolor, see #1

		if strings.Count(arg, ",") == 2 {
			rgbColor, err := rgbToColor(arg)
			if err != nil {
				return nil, fmt.Errorf("%s: %s is not a valid RGB tuple. Example: 25,200,150", flag, arg)
			}
			colors[i] = rgbColor
			continue
		}

		if flag == "recolor" && strings.Count(arg, ",") == 3 {
			rgbaColor, err := rgbaToColor(arg)
			if err != nil {
				return nil, fmt.Errorf("%s: %s is not a valid RGBA tuple. Example: 25,200,150,100", flag, arg)
			}
			colors[i] = rgbaColor
			continue
		}

		hexColor, err := hexToColor(arg)
		if err == nil {
			colors[i] = hexColor
			continue
		}

		n, err := strconv.Atoi(arg)
		if err == nil {
			if n > 255 || n < 0 {
				return nil, fmt.Errorf("%s: single numbers like %d must be in the range 0-255", flag, n)
			}
			colors[i] = color.NRGBA{uint8(n), uint8(n), uint8(n), 255}
			continue
		}

		htmlColor, ok := colornames.Map[strings.ToLower(arg)]
		if ok {
			colors[i] = color.NRGBAModel.Convert(htmlColor).(color.NRGBA)
			continue
		}

		return nil, fmt.Errorf("%s: %s not recognized as an RGB tuple, hex code, number 0-255, or SVG color name", flag, arg)
	}

	return colors, nil
}

// getInputImage takes an input image arg and returns an image that has
// modifications applied.
func getInputImage(arg string, c *cli.Context) (image.Image, error) {
	var img image.Image
	var err error

	if arg == "-" {
		img, err = imaging.Decode(os.Stdin, autoOrientation)
	} else {
		img, err = imaging.Open(arg, autoOrientation)
	}
	if err != nil {
		return nil, err
	}

	if width != 0 || height != 0 {
		// Box sampling is quick and fast, and better then others at downscaling
		// Downscaling will be a much more common use case for pre-dither scaling
		// then upscaling
		// https://pkg.go.dev/github.com/disintegration/imaging#ResampleFilter
		// https://en.wikipedia.org/wiki/Image_scaling#Box_sampling
		img = imaging.Resize(img, width, height, imaging.Box)
	}

	if grayscale {
		img = imaging.Grayscale(img)
	}
	if saturation != 0 {
		img = imaging.AdjustSaturation(img, saturation)
	}
	if contrast != 0 {
		img = imaging.AdjustContrast(img, contrast)
	}
	if brightness != 0 {
		img = imaging.AdjustBrightness(img, brightness)
	}

	return img, nil
}

// From dither library

func copyImage(dst draw.Image, src image.Image) {
	draw.Draw(dst, src.Bounds(), src, src.Bounds().Min, draw.Src)
}
func copyOfImage(img image.Image) *image.RGBA {
	dst := image.NewRGBA(img.Bounds())
	copyImage(dst, img)
	return dst
}

///////

// recolor will recolor the image pixels if necessary. It should be called
// before writing any image. It should only be given a dithered image.
// It will copy an image if it cannot draw on it.
//
// If the input image is *image.Paletted, the output will always be of that type too.
func recolor(src image.Image) image.Image {
	if len(recolorPalette) == 0 {
		return src
	}

	// Modified and returned value
	var img draw.Image

	// getRecolor takes an image color and returns the recolor one
	getRecolor := func(a color.Color) color.Color {
		// palette and recolorPalette are both NRGBA, so use that here too
		c := color.NRGBAModel.Convert(a).(color.NRGBA)

		for i := range palette {
			pc := palette[i].(color.NRGBA)
			if pc.R == c.R && pc.G == c.G && pc.B == c.B {
				// Colors match. Alpha is ignored because palette colors aren't
				// allowed alpha, so theirs will always be 255. While the image
				// might have a different alpha at that point
				return recolorPalette[i]
			}
		}
		// This should never happen
		return recolorPalette[0]
	}

	// Fast path for paletted images
	if p, ok := src.(*image.Paletted); ok {
		// For each color in the image palette, replace it with the equivalent
		// recolor palette color
		for i, c := range p.Palette {
			p.Palette[i] = getRecolor(c)
		}
		return p
	}

	var ok bool
	if img, ok = src.(draw.Image); !ok {
		// Can't be changed
		// Instead make a copy and recolor and return that
		img = copyOfImage(src)
	}

	// Swap each image pixel

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			// Image pixel -> convert to RGBA -> find recolor palette color using map
			// -> set color
			img.Set(x, y, getRecolor(img.At(x, y)))
		}
	}
	return img
}

// postProcImage post-processes the image, applying recolor and upscaling.
//
// If the input image is *image.Paletted, the output will always be of that type too.
func postProcImage(img image.Image) image.Image {
	img = recolor(img)

	if upscale == 1 {
		return img
	}

	var palette color.Palette
	if p, ok := img.(*image.Paletted); ok {
		palette = p.Palette
	}

	img = imaging.Resize(
		img,
		img.Bounds().Dx()*upscale,
		0,
		imaging.NearestNeighbor,
	)

	if len(palette) == 0 {
		return img
	}

	pi := image.NewPaletted(img.Bounds(), palette)
	copyImage(pi, img)
	return pi
}

// processImages dithers all the input images and writes them.
// It handles all image I/O.
func processImages(d *dither.Ditherer, c *cli.Context) error {
	outPath := globalFlag("out", c).(string)

	// Setup for if it's an animated GIF output
	// Overall adapted from:
	// https://github.com/makeworld-the-better-one/dither/blob/v2.0.0/examples/gif_animation.go

	isAnimGIF := len(inputImages) > 1 && outFormat == "gif" && !outIsDir

	var frames []*image.Paletted
	if isAnimGIF {
		frames = make([]*image.Paletted, len(inputImages))
	}

	var delays []int
	var animGIF gif.GIF
	if isAnimGIF {
		if !globalIsSet("fps", c) {
			return errors.New("output will be animated GIF, but --fps flag is not set")
		}

		delays = make([]int, len(inputImages))
		for i := range delays {
			// Round to the nearest possible frame rate supported by the GIF format
			// See for details: https://superuser.com/a/1449370
			// A rolling average is not done because it's harder to code and looks
			// bad: https://superuser.com/q/1459724
			//
			// Lowest allowed delay is 1, or 100 FPS.
			delays[i] = int(math.Max(math.Round(100.0/globalFlag("fps", c).(float64)), 1))
		}

		loopCount := int(globalFlag("loop", c).(uint))
		if loopCount == 1 {
			// Looping once is set using -1 in the image/gif library
			loopCount = -1
		} else if loopCount != 0 {
			// The CLI flag is equal to the number of times looped
			// But for gif.GIF.LoopCount, "the animation is looped LoopCount+1 times."
			loopCount -= 1
		}
		animGIF = gif.GIF{
			Image:     frames,
			Delay:     delays,
			LoopCount: loopCount,
		}
	}

	// Go through images and dither (and write if not an animated GIF)

	for i, inputPath := range inputImages {

		img, err := getInputImage(inputPath, c)
		if err != nil {
			return fmt.Errorf("error loading '%s': %w", inputPath, err)
		}

		if isAnimGIF {
			if i == 0 {
				// Use the config of the first image for the animated GIF
				var config image.Config
				frames[0], config = d.DitherPalettedConfig(img)
				frames[0] = postProcImage(frames[0]).(*image.Paletted)

				if len(recolorPalette) == 0 {
					animGIF.Config = config
				} else {
					// Same config as the Ditherer would give, but with the recolor palette
					animGIF.Config = image.Config{
						ColorModel: color.Palette(recolorPalette),
						Width:      frames[0].Bounds().Dx(),
						Height:     frames[0].Bounds().Dy(),
					}
				}
				continue
			}
			// Later frames
			if upscale == 1 && !img.Bounds().Eq(frames[0].Bounds()) {
				// Upscale check is needed because otherwise frames[0] will be upscaled and not match
				return fmt.Errorf(
					"image '%s' isn't the same size as '%s', all sizes must match to create an animated GIF",
					inputPath, inputImages[0],
				)
			}
			frames[i] = d.DitherPaletted(img)
			frames[i] = postProcImage(frames[i]).(*image.Paletted)

			// Do bounds check now, if it didn't happen before because of upscaling
			if upscale != 1 && !frames[i].Bounds().Eq(frames[0].Bounds()) {
				return fmt.Errorf(
					"image '%s' isn't the same size as '%s', all sizes must match to create an animated GIF",
					inputPath, inputImages[0],
				)
			}
			continue
		}

		// Not an animated GIF
		// Write out the image now
		// (partially copied below, outside the loop)

		var file io.WriteCloser
		var path string

		if outPath == "-" {
			file = os.Stdout
			path = "stdout"
		} else {
			if outIsDir {
				// Inside output directory
				// Same name as input file but potentially different extension
				path = filepath.Join(
					outPath,
					strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))+"."+outFormat,
				)
			} else {
				// Output file path
				path = outPath
			}

			file, err = os.OpenFile(path, outFileFlags, 0644)
			if err != nil {
				return fmt.Errorf("'%s': %w", path, err)
			}
		}

		if outFormat == "png" {
			img = postProcImage(d.Dither(img))
			err = (&png.Encoder{CompressionLevel: compLevel}).Encode(file, img)
			if err != nil {
				defer file.Close() // Keep (possibly stdout) open to write error messages then close
				return fmt.Errorf("error writing PNG to '%s': %w", path, err)
			}
			file.Close()
		} else {
			// Output static GIF
			// Adapted from:
			// https://github.com/makeworld-the-better-one/dither/blob/v2.0.0/examples/gif_image.go

			if !postProcNeeded {
				// No post
				// GIF encoder calls the ditherer
				err = gif.Encode(
					file, img,
					&gif.Options{
						NumColors: len(palette),
						Quantizer: d,
						Drawer:    d,
					},
				)
			} else {
				// Dither and post-process first, and use recolor palette if needed
				// The gif package will not change the image if it's *image.Paletted
				// So even though Drawer is not set to the ditherer it'll be fine,
				// and the default FloydSteinberg Drawer won't be used

				img = postProcImage(d.DitherPaletted(img))

				var quantizer draw.Quantizer
				if len(recolorPalette) == 0 {
					quantizer = d
				} else {
					quantizer = &fakeQuantizer{recolorPalette}
				}
				err = gif.Encode(
					file, img,
					&gif.Options{
						NumColors: len(recolorPalette),
						Quantizer: quantizer,
					},
				)
			}
			if err != nil {
				defer file.Close()
				return fmt.Errorf("error writing GIF to '%s': %w", path, err)
			}
			file.Close()
		}
	}

	// Either all images have been written and everything is done, or the animated GIF
	// needs to be saved.

	if !isAnimGIF {
		return nil
	}

	// Partially copied from above

	var file io.WriteCloser
	var path string
	var err error

	if outPath == "-" {
		file = os.Stdout
		path = "stdout"
	} else {
		// Output file path
		path = outPath
		file, err = os.OpenFile(path, outFileFlags, 0644)
		if err != nil {
			return fmt.Errorf("'%s': %w", path, err)
		}
	}

	err = gif.EncodeAll(file, &animGIF)
	if err != nil {
		defer file.Close()
		return fmt.Errorf("error writing GIF to '%s': %w", path, err)
	}
	file.Close()
	return nil
}
