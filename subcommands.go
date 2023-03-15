package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/makeworld-the-better-one/dither/v2"
	"github.com/urfave/cli/v2"
)

const (
	unsupportedFormat string = "'%s' is an unsupported format, only 'png' or 'gif' are accepted"
)

var (
	// palette stores the palette colors. It's set after pre-processing.
	// Guaranteed to only hold color.NRGBA.
	palette []color.Color

	// recolorPalette stores the recolor palette colors. It's set after pre-processing.
	// Guaranteed to only hold color.NRGBA.
	recolorPalette []color.Color

	grayscale bool

	// Range -100,100

	saturation float64
	brightness float64
	contrast   float64

	autoOrientation imaging.DecodeOption

	inputImages []string
	outFormat   string // "png" or "gif"
	outIsDir    bool

	compLevel png.CompressionLevel

	outFileFlags int // For os.OpenFile

	width  int
	height int
	// upscale will always be 1 or above
	upscale int

	ditherer *dither.Ditherer

	// range [-1, 1]
	strength float32

	// Is post-processing needed?
	postProcNeeded bool
)

// preProcess is automatically called by the app before anything else.
// It's run in the global context.
func preProcess(c *cli.Context) error {
	runtime.GOMAXPROCS(int(c.Uint("threads")))

	var err error

	saturation, err = parsePercentArg(c.String("saturation"), false)
	if err != nil {
		return fmt.Errorf("saturation: %w", err)
	}
	if saturation <= -100 {
		grayscale = true
		saturation = 0
	}
	brightness, err = parsePercentArg(c.String("brightness"), false)
	if err != nil {
		return fmt.Errorf("brightness: %w", err)
	}
	contrast, err = parsePercentArg(c.String("contrast"), false)
	if err != nil {
		return fmt.Errorf("contrast: %w", err)
	}

	autoOrientation = imaging.AutoOrientation(!c.Bool("no-exif-rotation"))

	inputImages = make([]string, 0)
	for _, path := range c.StringSlice("in") {
		if strings.Contains(path, "*") {
			// Parse as glob
			paths, err := filepath.Glob(path)
			if err != nil {
				return fmt.Errorf("bad glob pattern '%s': %w", path, err)
			}
			inputImages = append(inputImages, paths...)
		} else {
			inputImages = append(inputImages, path)
		}
	}

	palette, err = parseColors("palette", c)
	if err != nil {
		return err
	}
	if len(palette) < 2 {
		return errors.New("the palette must have at least two colors")
	}

	if c.String("recolor") != "" {
		recolorPalette, err = parseColors("recolor", c)
		if err != nil {
			return err
		}
		if len(recolorPalette) != len(palette) {
			return errors.New("recolor palette must have the same number of colors as the initial palette")
		}
	}

	// Check if palette is grayscale and make image grayscale
	// Or if the user forces it

	grayscale = true
	if !c.Bool("grayscale") {
		// Grayscale isn't specified by the user
		// So check to see if palette is grayscale
		for _, c := range palette {
			r, g, b, _ := c.RGBA()
			if r != g || g != b {
				grayscale = false
				break
			}
		}
	}

	formatVal := c.String("format")
	if formatVal != "png" && formatVal != "gif" {
		return fmt.Errorf(unsupportedFormat, formatVal)
	}

	// Figure out output format

	outVal := c.String("out")

	if outVal == "-" {
		// Outputting to stdout, so just use whatever the flag is
		outFormat = formatVal
	} else {
		// Outputting to dir or file

		outFI, err := os.Stat(outVal)

		if err == nil && outFI.IsDir() {
			// Exists and is a directory
			// Just use what the flag is
			outFormat = formatVal
			outIsDir = true

		} else {
			// Outputting to file, that already exists
			// Or something that doesn't exist - assumed to be a file

			if !c.IsSet("format") {
				// Format wasn't set, so ignore default value of "png"
				// Try to figure out format from output filename
				ext := strings.TrimPrefix(filepath.Ext(outVal), ".")
				if ext == "png" || ext == "gif" {
					// Acceptable extension
					outFormat = ext
				} else if ext == "" {
					// No extension, use default format
					outFormat = "png"
				} else {
					// Unsupported extension and no format flag override
					return fmt.Errorf(unsupportedFormat, ext)
				}
			} else {
				// Format flag was set, so ignore what the file looks like
				outFormat = formatVal
			}
		}

	}

	// Multiple input images are only valid if the output is GIF,
	// or if the output points to a directory.
	if len(inputImages) > 1 && (outFormat != "gif" && !outIsDir) {
		return fmt.Errorf("multiple input images are only allowed if the output format is GIF, or an existing directory")
	}

	if outFormat == "gif" && len(palette) > 256 {
		return errors.New("the GIF format only supports 256 colors or less in the palette")
	}

	// Set PNG compression type

	switch c.String("compression") {
	case "default":
		compLevel = png.DefaultCompression
	case "no":
		compLevel = png.NoCompression
	case "speed":
		compLevel = png.BestSpeed
	case "size":
		compLevel = png.BestCompression
	default:
		return fmt.Errorf("invalid compression type '%s'", c.String("compression"))
	}

	if c.Bool("no-overwrite") {
		outFileFlags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	} else {
		outFileFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	// Set here for convenience
	width = int(c.Uint("width"))
	height = int(c.Uint("height"))
	upscale = int(c.Uint("upscale"))
	if upscale == 0 {
		// Invalid
		upscale = 1
	}

	ditherer = dither.NewDitherer(palette)

	tmp, err := parsePercentArg(c.String("strength"), true)
	if err != nil {
		return fmt.Errorf("strength: %w", err)
	}
	strength = float32(tmp)
	if strength == 0 {
		// Ignore
		strength = 1
	}

	if len(recolorPalette) != 0 || upscale > 1 {
		postProcNeeded = true
	}

	return nil
}

func random(c *cli.Context) error {
	args := parseArgs(c.Args().Slice(), " ,")

	// Manually parse out --seed, -s flag
	// The manual parsing is done to allow for numbers that start with a negative
	// which would otherwise be interpreted as flags

	seedIsSet := false
	var seed int64

	if len(args) >= 1 {
		if args[0] == "--seed" || args[0] == "-s" {
			if len(args) >= 2 {
				// Parse and set seed value

				var err error
				seed, err = strconv.ParseInt(args[1], 10, 64)
				if err != nil {
					return fmt.Errorf("couldn't parse seed value: %w", err)
				}
				seedIsSet = true
				args = args[2:]
			} else {
				// Seed flag but no value after it
				return errors.New("no value after seed flag")
			}
		} else if args[0] == "--help" || args[0] == "-h" {
			// Display the help
			return cli.ShowCommandHelp(c, "random")
		}
	}

	if len(args) != 2 && len(args) != 6 {
		return errors.New("random needs 2 or 6 arguments")
	}

	floatArgs := make([]float32, len(args))
	for i, arg := range args {
		f64, err := parsePercentArg(arg, true)
		if err != nil {
			return err
		}
		floatArgs[i] = float32(f64)
	}

	if seedIsSet {
		rand.Seed(seed)
	} else {
		// Seed with something that won't repeat next use
		rand.Seed(time.Now().UnixNano())
	}

	if len(floatArgs) == 2 {
		if grayscale {
			ditherer.Mapper = dither.RandomNoiseGrayscale(floatArgs[0], floatArgs[1])
		} else {
			// Use the two arguments for all channels
			ditherer.Mapper = dither.RandomNoiseRGB(floatArgs[0], floatArgs[1], floatArgs[0], floatArgs[1], floatArgs[0], floatArgs[1])
		}
	} else {
		ditherer.Mapper = dither.RandomNoiseRGB(floatArgs[0], floatArgs[1], floatArgs[2], floatArgs[3], floatArgs[4], floatArgs[5])
	}
	if seedIsSet {
		// Make deterministic
		ditherer.SingleThreaded = true
	}

	err := processImages(ditherer, c)
	if err != nil {
		return err
	}
	return nil
}

func bayer(c *cli.Context) error {
	args := parseArgs(c.Args().Slice(), " ,x")

	if len(args) != 2 {
		return errors.New("bayer needs 2 arguments exactly. Example: 4x4")
	}

	uintArgs := make([]uint, 2)
	for i, arg := range args {
		u64, err := strconv.ParseUint(arg, 10, 0)
		if err != nil {
			return err
		}
		uintArgs[i] = uint(u64)
	}

	// Validate args to prevent dither.Bayer from panicking

	x, y := uintArgs[0], uintArgs[1]
	if x == 0 || y == 0 {
		return errors.New("neither dimension can be 0")
	}
	if x == 1 && y == 1 {
		return errors.New("a 1x1 matrix will not dither the image")
	}
	if ((x&(x-1)) != 0 || (y&(y-1)) != 0) && // Power of two?
		!((x == 3 && y == 3) || (x == 5 && y == 3) || (x == 3 && y == 5)) { // Exceptions
		// Not a power of two, and not an exception
		return errors.New("both dimensions must be powers of two")
	}

	ditherer.Mapper = dither.Bayer(x, y, strength)

	err := processImages(ditherer, c)
	if err != nil {
		return err
	}
	return nil
}

var odmName = map[string]dither.OrderedDitherMatrix{
	"clustereddot4x4":            dither.ClusteredDot4x4,
	"clustereddotdiagonal8x8":    dither.ClusteredDotDiagonal8x8,
	"vertical5x3":                dither.Vertical5x3,
	"horizontal3x5":              dither.Horizontal3x5,
	"clustereddotdiagonal6x6":    dither.ClusteredDotDiagonal6x6,
	"clustereddotdiagonal8x8_2":  dither.ClusteredDotDiagonal8x8_2,
	"clustereddotdiagonal16x16":  dither.ClusteredDotDiagonal16x16,
	"clustereddot6x6":            dither.ClusteredDot6x6,
	"clustereddotspiral5x5":      dither.ClusteredDotSpiral5x5,
	"clustereddothorizontalline": dither.ClusteredDotHorizontalLine,
	"clustereddotverticalline":   dither.ClusteredDotVerticalLine,
	"clustereddot8x8":            dither.ClusteredDot8x8,
	"clustereddot6x6_2":          dither.ClusteredDot6x6_2,
	"clustereddot6x6_3":          dither.ClusteredDot6x6_3,
	"clustereddotdiagonal8x8_3":  dither.ClusteredDotDiagonal8x8_3,
}

func odm(c *cli.Context) error {
	args := c.Args().Slice()

	if len(args) != 1 {
		return errors.New("odm only accepts one argument")
	}

	var matrix dither.OrderedDitherMatrix

	matrix, ok := odmName[strings.ReplaceAll(strings.ToLower(args[0]), "-", "_")]
	if !ok {
		// Either inline JSON, path to file, or an error
		err := json.Unmarshal([]byte(args[0]), &matrix)
		if err != nil {
			bytes, err := ioutil.ReadFile(args[0])
			if err != nil {
				return errors.New("couldn't process argument as matrix name, inline JSON, or path to accessible JSON file")
			}
			err = json.Unmarshal(bytes, &matrix)
			if err != nil {
				return errors.New("couldn't process argument as matrix name, inline JSON, or path to accessible JSON file")
			}
		}

		// Validate matrix

		if matrix.Max == 0 {
			return errors.New("the max value of the matrix cannot be 0")
		}
		if len(matrix.Matrix) == 0 {
			return errors.New("matrix is empty")
		}
		// Is it rectangular?
		width := len(matrix.Matrix[0])
		if width == 0 {
			return errors.New("matrix has empty row")
		}
		for _, row := range matrix.Matrix {
			if len(row) != width {
				return errors.New("matrix is not rectangular, all rows must be the same length")
			}
		}
	}

	ditherer.Mapper = dither.PixelMapperFromMatrix(matrix, strength)

	err := processImages(ditherer, c)
	if err != nil {
		return err
	}
	return nil
}

var edmName = map[string]dither.ErrorDiffusionMatrix{
	"simple2d":            dither.Simple2D,
	"floydsteinberg":      dither.FloydSteinberg,
	"falsefloydsteinberg": dither.FalseFloydSteinberg,
	"jarvisjudiceninke":   dither.JarvisJudiceNinke,
	"atkinson":            dither.Atkinson,
	"stucki":              dither.Stucki,
	"burkes":              dither.Burkes,
	"sierra":              dither.Sierra,
	"sierra3":             dither.Sierra3,
	"tworowsierra":        dither.TwoRowSierra,
	"sierralite":          dither.SierraLite,
	"sierra2_4a":          dither.Sierra2_4A,
	"stevenpigeon":        dither.StevenPigeon,
}

func edm(c *cli.Context) error {
	args := c.Args().Slice()

	if len(args) != 1 {
		return errors.New("edm only accepts one argument")
	}

	var matrix dither.ErrorDiffusionMatrix

	matrix, ok := edmName[strings.ReplaceAll(strings.ToLower(args[0]), "-", "_")]
	if !ok {
		// Either inline JSON, path to file, or an error
		err := json.Unmarshal([]byte(args[0]), &matrix)
		if err != nil {
			bytes, err := ioutil.ReadFile(args[0])
			if err != nil {
				return errors.New("couldn't process argument as matrix name, inline JSON, or path to accessible JSON file")
			}
			err = json.Unmarshal(bytes, &matrix)
			if err != nil {
				return errors.New("couldn't process argument as matrix name, inline JSON, or path to accessible JSON file")
			}
		}

		// Validate matrix

		if len(matrix) == 0 {
			return errors.New("matrix is empty")
		}
		// Is it rectangular?
		width := len(matrix[0])
		if width == 0 {
			return errors.New("matrix has empty row")
		}
		for _, row := range matrix {
			if len(row) != width {
				return errors.New("matrix is not rectangular, all rows must be the same length")
			}
		}
	}

	ditherer.Matrix = dither.ErrorDiffusionStrength(matrix, strength)
	if c.Bool("serpentine") {
		ditherer.Serpentine = true
	}

	err := processImages(ditherer, c)
	if err != nil {
		return err
	}
	return nil
}
