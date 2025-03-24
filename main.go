package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"strings"

	"github.com/urfave/cli/v3"
)

func main() {
	(&cli.Command{
		Name:  "uvpad",
		Usage: "Texture dilating tool",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Value: "",
				Usage: "Output image file",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				fmt.Println("Usage: uvpad <input image>")
				return nil
			}
			input := cmd.Args().Get(0)

			ext := path.Ext(input)
			output := strings.TrimSuffix(input, ext) + "_padded" + ext
			if cmd.String("output") != "" {
				output = cmd.String("output")
			}

			return run(input, output)
		},
	}).Run(context.Background(), os.Args)
}

func run(input, output string) error {
	inputFile, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	inputImage, err := png.Decode(inputFile)
	if err != nil {
		return fmt.Errorf("failed to decode input image: %w", err)
	}

	data := process(inputImage)

	err = save(output, data)
	if err != nil {
		return fmt.Errorf("failed to save output image: %w", err)
	}
	return nil
}

func process(input image.Image) image.Image {
	bounds := input.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rgba := image.NewRGBA(bounds)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			rgba.Set(x, y, input.At(x, y))
		}
	}

	output := image.NewRGBA(bounds)
	copy(output.Pix, rgba.Pix)

	remaining := 0
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			_, _, _, alpha := rgba.At(x, y).RGBA()
			if alpha != 0xffff {
				remaining++
			}
		}
	}

	passes := 0
	for remaining > 0 {
		fmt.Printf("Pass %d: %d remaining\n", passes, remaining)
		passes++

		tempImg := image.NewRGBA(bounds)
		copy(tempImg.Pix, output.Pix)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				pixelIdx := y*output.Stride + x*4
				alpha := output.Pix[pixelIdx+3]

				if alpha != 255 {
					var r, g, b uint32
					var count uint32

					neighbours := []struct{ dx, dy int }{
						{-1, 0}, {1, 0}, {0, -1}, {0, 1},
					}

					for _, n := range neighbours {
						nx, ny := x+n.dx, y+n.dy
						if nx >= 0 && nx < width && ny >= 0 && ny < height {
							nr, ng, nb, na := rgba.At(nx, ny).RGBA()
							if na == 0xffff {
								r += nr
								g += ng
								b += nb
								count++
							}
						}
					}

					if count > 0 {
						tempImg.Pix[pixelIdx] = uint8((r / count) >> 8)
						tempImg.Pix[pixelIdx+1] = uint8((g / count) >> 8)
						tempImg.Pix[pixelIdx+2] = uint8((b / count) >> 8)
						tempImg.Pix[pixelIdx+3] = 255 // Make fully opaque
						remaining--
					}
				}
			}
			if y%20 == 0 {
				progress := float64(height*width-remaining) / float64(height*width)
				fmt.Printf("Progress: %.1f%%\r", progress*100)
			}
		}

		copy(output.Pix, tempImg.Pix)
		copy(rgba.Pix, output.Pix)
	}

	return output
}

func save(output string, data image.Image) error {
	outputFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	err = png.Encode(outputFile, data)
	if err != nil {
		return fmt.Errorf("failed to encode output image: %w", err)
	}
	return nil
}
