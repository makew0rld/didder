package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

// Set by compiler, see Makefile
var (
	version = "v1.0.0"
	commit  = "unknown"
	builtBy = "unknown"
)

func main() {
	var (
		matrixTypesDesc = "This only takes one argument, but there a few types available:\n" +
			helpList(1,
				"A preprogrammed matrix name",
				"Inline JSON of a custom matrix",
				"Or a path to JSON for your custom matrix. '-' means stdin.",
			) + "\n"

		decimalOrPercent = ", using a decimal or percentage."

		caseInsensitiveNames = "\nTheir names are case-insensitive, and hyphens and underscores are treated the same."

		description = `
Colors (for --palette and --recolor) are entered as a single quoted argument.
They can be separated by spaces and commas. Colors can be formatted as hex
codes (case-insensitive, with or without the '#'), a single number from 0-255
for grayscale, or a color name from the SVG 1.1 spec (aka the HTML or W3C
color names). All colors are interpreted in the sRGB colorspace.

Color names: https://www.w3.org/TR/SVG11/types.html#ColorKeywords

Images are converted to grayscale automatically if the palette is grayscale.
This produces more correct results.

Decimal range is -1.0 to 1.0. Percentage range is -100% or 100%.

The input file path can also be parsed as a glob. This will only happen if the
path contains an asterisk. For example -i '*.jpg' will select all the .jpg
files in the current directory as input. See this page for more info on glob
pattern matching: https://golang.org/pkg/path/filepath/#Match`
	)

	app := &cli.App{
		Name:                   "didder",
		Usage:                  "dither images with a variety of algorithms and processing options.",
		Description:            description,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "strength",
				Aliases: []string{"s"},
				Usage:   "set strength of dithering" + decimalOrPercent + " Exceeding the range will work. A zero value will be ignored.",
			},
			&cli.UintFlag{
				Name:    "threads",
				Aliases: []string{"j"},
				Usage:   "set number of threads for ordered dithering",
			},
			&cli.StringFlag{
				Name:     "palette",
				Aliases:  []string{"p"},
				Usage:    "set color palette used for dithering",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "grayscale",
				Usage: "make input image(s) grayscale before dithering",
			},
			&cli.StringFlag{
				Name:  "saturation",
				Usage: "change input image(s) saturation before dithering" + decimalOrPercent,
			},
			&cli.StringFlag{
				Name:  "brightness",
				Usage: "change input image(s) brightness before dithering" + decimalOrPercent,
			},
			&cli.StringFlag{
				Name:  "contrast",
				Usage: "change input image(s) contrast before dithering" + decimalOrPercent,
			},
			&cli.StringFlag{
				Name:    "recolor",
				Aliases: []string{"r"},
				Usage:   "set color palette used for replacing the dithered color palette after dithering",
			},
			&cli.BoolFlag{
				Name:  "no-exif-rotation",
				Usage: "disable using the EXIF rotation flag to rotate the image before processing",
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage:   "set output file format. Valid options are png and gif. It will auto detect from filename when possible.",
				Value:   "png",
			},
			&cli.StringFlag{
				Name:     "out",
				Aliases:  []string{"o"},
				Usage:    "set output file path or directory",
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:     "in",
				Aliases:  []string{"i"},
				Usage:    "set input file path, specify multiple times for multiple inputs",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "no-overwrite",
				Usage: "the command will stop before overwriting an existing file. Files written before this one was encountered will stay in place.",
			},
			&cli.StringFlag{
				Name:    "compression",
				Aliases: []string{"c"},
				Usage:   "PNG compression type. Options: 'default', 'no', 'speed', 'size'",
				Value:   "default",
			},
			&cli.Float64Flag{
				Name:  "fps",
				Usage: "set frames per second for animated GIF output",
			},
			&cli.UintFlag{
				Name:  "loop",
				Usage: "number of times the animated GIF output should loop, 0 is infinite",
			},
			&cli.UintFlag{
				Name:    "width",
				Aliases: []string{"x"},
				Usage:   "set the width the input image(s) will be resized to, BEFORE dithering. Aspect ratio will be maintained if --height is not specified",
			},
			&cli.UintFlag{
				Name:    "height",
				Aliases: []string{"y"},
				Usage:   "set the height the input image(s) will be resized to, BEFORE dithering. Aspect ratio will be maintained if --width is not specified",
			},
			&cli.UintFlag{
				Name:    "upscale",
				Aliases: []string{"u"},
				Usage:   "scale image up AFTER dithering. So '2' will make the output 2 times as big as the input. Integer only.",
				Value:   1,
			},
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "get version info",
			},
		},
		Commands: []*cli.Command{
			{
				Name:        "random",
				Usage:       "grayscale and RGB random dithering",
				Description: "Specify two arguments (min and max) for RGB or grayscale, or 6 (min/max for each channel) to control each RGB channel.\nArguments can be separated by commas or spaces. -0.5,0.5 is a good default.",
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:    "seed",
						Aliases: []string{"s"},
						Usage:   "set the seed for randomization. This will also only use one thread, to keep output deterministic",
					},
				},
				UseShortOptionHandling: true,
				Action:                 random,
				SkipFlagParsing:        true, // Allow for numbers that start with a negative
			},
			{
				Name:                   "bayer",
				Usage:                  "Bayer matrix ordered dithering",
				Description:            "Two arguments, for the X and Y dimension of the matrix. They can be separated by a space, comma, or 'x'.\nBoth arguments must be a power of two, with the exception of: 3x5, 5x3, and 3x3.",
				UseShortOptionHandling: true,
				Action:                 bayer,
			},
			{
				Name:  "odm",
				Usage: "Ordered Dither Matrix",
				Description: "Select or provide an ordered dithering matrix. " + matrixTypesDesc +
					"Here are all the built-in ordered dithering matrices. You can find details on these matrices here:\nhttps://github.com/makeworld-the-better-one/dither/blob/v2.0.0/ordered_ditherers.go\n\n" +
					helpList(1,
						"ClusteredDot4x4",
						"ClusteredDotDiagonal8x8",
						"Vertical5x3",
						"Horizontal3x5",
						"ClusteredDotDiagonal6x6",
						"ClusteredDotDiagonal8x8_2",
						"ClusteredDotDiagonal16x16",
						"ClusteredDot6x6",
						"ClusteredDotSpiral5x5",
						"ClusteredDotHorizontalLine",
						"ClusteredDotVerticalLine",
						"ClusteredDot8x8",
						"ClusteredDot6x6_2",
						"ClusteredDot6x6_3",
						"ClusteredDotDiagonal8x8_3",
					) + caseInsensitiveNames,
				UseShortOptionHandling: true,
				Action:                 odm,
			},
			{
				Name:  "edm",
				Usage: "Error Diffusion Matrix",
				Description: "Select or provide an error diffusion matrix. " + matrixTypesDesc +
					"Here are all the built-in error diffusion matrices. You can find details on these matrices here:\nhttps://github.com/makeworld-the-better-one/dither/blob/v2.0.0/error_diffusers.go\n\n" +
					helpList(1,
						"Simple2D",
						"FloydSteinberg",
						"FalseFloydSteinberg",
						"JarvisJudiceNinke",
						"Atkinson",
						"Stucki",
						"Burkes",
						"Sierra (or Sierra3)",
						"TwoRowSierra (or Sierra2)",
						"SierraLite (or Sierra2_4A)",
						"StevenPigeon",
					) + caseInsensitiveNames,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "serpentine",
						Aliases: []string{"s"},
						Usage:   "enable serpentine dithering",
					},
				},
				UseShortOptionHandling: true,
				Action:                 edm,
			},
		},
		Before: preProcess,
		Action: func(c *cli.Context) error {
			return errors.New("no command specified")
		},
	}

	// Handle version flag
	if len(os.Args) == 2 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Println("didder", version)
		fmt.Println("Commit:", commit)
		fmt.Println("Built by:", builtBy)
		return
	}

	// Hack around issue where required flags are still required even for help
	// https://github.com/urfave/cli/issues/1247
	if len(os.Args) == 3 {
		if os.Args[1] == "h" || os.Args[1] == "help" {
			// Like: didder help bayer
			for _, c := range app.Commands {
				if c.Name == os.Args[2] {
					cli.HelpPrinter(os.Stdout, cli.CommandHelpTemplate, c)
					return
				}
			}
			fmt.Println("no command with that name")
			os.Exit(1)
		} else if os.Args[len(os.Args)-1] == "-h" || os.Args[len(os.Args)-1] == "--help" {
			// Like: didder bayer --help
			for _, c := range app.Commands {
				if c.Name == os.Args[1] {
					cli.HelpPrinter(os.Stdout, cli.CommandHelpTemplate, c)
					return
				}
			}
			fmt.Println("no command with that name")
			os.Exit(1)
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		if len(os.Args) == 1 {
			// Just ran the command with no flags
			return
		}
		fmt.Println(err)
		os.Exit(1)
	}
}

// helpList creates an indented list for the CLI help/usage info.
func helpList(level int, items ...string) string {
	ret := ""
	for _, item := range items {
		ret += strings.Repeat("  ", level) + "- " + item + "\n"
	}
	return ret
}
