package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

// Set by compiler, see Makefile
var (
	version = "v1.2.0"
	commit  = "unknown"
	builtBy = "unknown"
)

func main() {

	app := &cli.App{
		Name:                   "didder",
		Usage:                  "dither images with a variety of algorithms and processing options.",
		Description:            "didder dithers images.\n\nRun `man didder` for more information, or view the manual online:\nhttps://github.com/makeworld-the-better-one/didder/blob/main/MANPAGE.md",
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "strength",
				Aliases: []string{"s"},
			},
			&cli.UintFlag{
				Name:    "threads",
				Aliases: []string{"j"},
			},
			&cli.StringFlag{
				Name:     "palette",
				Aliases:  []string{"p"},
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "grayscale",
				Aliases: []string{"g"},
			},
			&cli.StringFlag{
				Name: "saturation",
			},
			&cli.StringFlag{
				Name: "brightness",
			},
			&cli.StringFlag{
				Name: "contrast",
			},
			&cli.StringFlag{
				Name:    "recolor",
				Aliases: []string{"r"},
			},
			&cli.BoolFlag{
				Name: "no-exif-rotation",
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Value:   "png",
			},
			&cli.StringFlag{
				Name:     "out",
				Aliases:  []string{"o"},
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:     "in",
				Aliases:  []string{"i"},
				Required: true,
			},
			&cli.BoolFlag{
				Name: "no-overwrite",
			},
			&cli.StringFlag{
				Name:    "compression",
				Aliases: []string{"c"},
				Value:   "default",
			},
			&cli.Float64Flag{
				Name: "fps",
			},
			&cli.UintFlag{
				Name:    "loop",
				Aliases: []string{"l"},
			},
			&cli.UintFlag{
				Name:    "width",
				Aliases: []string{"x"},
			},
			&cli.UintFlag{
				Name:    "height",
				Aliases: []string{"y"},
			},
			&cli.UintFlag{
				Name:    "upscale",
				Aliases: []string{"u"},
				Value:   1,
			},
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"v"},
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "random",
				Usage: "grayscale and RGB random dithering",
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:    "seed",
						Aliases: []string{"s"},
					},
				},
				UseShortOptionHandling: true,
				Action:                 random,
				SkipFlagParsing:        true, // Allow for numbers that start with a negative
			},
			{
				Name:                   "bayer",
				Usage:                  "Bayer matrix ordered dithering",
				UseShortOptionHandling: true,
				Action:                 bayer,
			},
			{
				Name:                   "odm",
				Usage:                  "Ordered Dither Matrix",
				UseShortOptionHandling: true,
				Action:                 odm,
			},
			{
				Name:  "edm",
				Usage: "Error Diffusion Matrix",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "serpentine",
						Aliases: []string{"s"},
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
