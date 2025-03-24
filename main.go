package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

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
			&cli.BoolFlag{
				Name:  "slower",
				Value: false,
				Usage: "If false, use the paint.net algorithm instead of GIMP UVPad algorithm",
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

			start := time.Now()

			err := run(input, output, cmd.Bool("slower"))
			if err != nil {
				return err
			}

			executionTime := time.Since(start)
			fmt.Printf("Execution time: %v\n", executionTime)

			fmt.Println("Saved padded image to", output)

			return nil
		},
	}).Run(context.Background(), os.Args)
}

func run(input, output string, slower bool) error {
	inputFile, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	inputImage, err := png.Decode(inputFile)
	if err != nil {
		return fmt.Errorf("failed to decode input image: %w", err)
	}

	var data image.Image
	if slower {
		data = process_gimp_alg(inputImage)
	} else {
		data = process_paint_net_alg(inputImage)
	}

	err = save(output, data)
	if err != nil {
		return fmt.Errorf("failed to save output image: %w", err)
	}
	return nil
}

type Point struct {
	x, y int
}

func process_paint_net_alg(input image.Image) image.Image {
	bounds := input.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	output := image.NewRGBA(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			output.Set(x, y, input.At(x, y))
		}
	}

	opaqueMask := make([]bool, width*height)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			_, _, _, alpha := input.At(x, y).RGBA()
			if alpha == 0xffff {
				r, g, b, _ := input.At(x, y).RGBA()
				output.Set(x, y, color.RGBA{
					uint8(r),
					uint8(g),
					uint8(b),
					255,
				})
				opaqueMask[y*width+x] = true
			}
		}
	}

	nearest := jumpFlood(width, height, opaqueMask)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			idx := y*width + x
			_, _, _, alpha := input.At(x, y).RGBA()

			if alpha != 0xffff {
				point := nearest[idx]
				if point.x != -1 && point.y != -1 {
					r, g, b, _ := input.At(point.x, point.y).RGBA()
					output.Set(x, y, color.RGBA{
						uint8(r >> 8),
						uint8(g >> 8),
						uint8(b >> 8),
						255,
					})
				} else {
					output.Set(x, y, color.RGBA{0, 0, 0, 0})
				}
			}
		}
	}

	return output
}

func jumpFlood(width, height int, opaqueMask []bool) []Point {
	distances := make([]float64, width*height)
	nearest := make([]Point, width*height)

	maxDistance := float64(width*width + height*height)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			idx := y*width + x
			if opaqueMask[idx] {
				distances[idx] = 0
				nearest[idx] = Point{x, y}
			} else {
				distances[idx] = maxDistance
				nearest[idx] = Point{-1, -1}
			}
		}
	}

	numCpu := runtime.NumCPU()
	maxSteps := int(math.Ceil(math.Log2(float64(math.Max(float64(width), float64(height)))))) * 2
	for step := 1; step < maxSteps; step++ {
		var wg sync.WaitGroup
		chunkSize := height / numCpu
		if chunkSize == 0 {
			chunkSize = 1
		}

		distancesCopy := make([]float64, len(distances))
		copy(distancesCopy, distances)

		nearestCopy := make([]Point, len(nearest))
		copy(nearestCopy, nearest)

		for i := 0; i < numCpu; i++ {
			wg.Add(1)
			start := i * chunkSize
			end := (i + 1) * chunkSize
			if i == numCpu-1 {
				end = height
			}

			go func(start, end int) {
				defer wg.Done()
				processJumpFlood(width, height, distancesCopy, nearestCopy, distances, nearest, step, start, end)
			}(start, end)
		}

		wg.Wait()
	}

	return nearest
}

func processJumpFlood(width, height int, distancesCopy []float64, nearestCopy []Point, distances []float64, nearest []Point, step, start, end int) {
	neighbours := []struct{ dx, dy int }{
		{-step, -step}, {0, -step}, {step, -step},
		{-step, 0}, {step, 0},
		{-step, step}, {0, step}, {step, step},
	}

	for y := start; y < end; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			bestDistance := distancesCopy[idx]

			for _, neighbour := range neighbours {
				nx, ny := x+neighbour.dx, y+neighbour.dy
				if nx >= 0 && nx < width && ny >= 0 && ny < height {
					neighbourIdx := ny*width + nx

					if nearestCopy[neighbourIdx].x != -1 && nearestCopy[neighbourIdx].y != -1 {
						npx, npy := nearestCopy[neighbourIdx].x, nearestCopy[neighbourIdx].y
						dx := float64(x - npx)
						dy := float64(y - npy)
						distance := dx*dx + dy*dy

						if distance < bestDistance {
							distances[idx] = distance
							nearest[idx] = nearestCopy[neighbourIdx]
							bestDistance = distance
						}
					}
				}
			}
		}
	}
}

func process_gimp_alg(input image.Image) image.Image {
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
