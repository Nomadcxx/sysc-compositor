package ui

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// GradientImage samples Niri's CSS-angle gradient in layout coordinates.
// The caller bounds the allocation before requesting a raster.
func GradientImage(bounds, area image.Rectangle, gradient config.Gradient) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	angle := float64(gradient.Angle-90) * math.Pi / 180
	dx, dy := math.Cos(angle), math.Sin(angle)
	length := math.Abs(float64(area.Dx())*dx) + math.Abs(float64(area.Dy())*dy)
	if length == 0 {
		return img
	}
	cx, cy := float64(area.Min.X+area.Max.X)/2, float64(area.Min.Y+area.Max.Y)/2
	mix := gradientMixer(gradient)
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			t := .5 + ((float64(bounds.Min.X+x)+.5-cx)*dx+(float64(bounds.Min.Y+y)+.5-cy)*dy)/length
			img.SetRGBA(x, y, mix(min(max(t, 0), 1)))
		}
	}
	return img
}

func gradientMixer(g config.Gradient) func(float64) color.RGBA {
	fields := strings.Fields(g.Interpolation)
	space, hue := "srgb", "shorter"
	if len(fields) > 0 {
		space = fields[0]
	}
	if len(fields) > 1 {
		hue = fields[1]
	}
	a, b := [3]float64{g.From.R, g.From.G, g.From.B}, [3]float64{g.To.R, g.To.G, g.To.B}
	if space != "srgb" {
		for i := range 3 {
			a[i] = math.Pow(a[i], 2.2)
			b[i] = math.Pow(b[i], 2.2)
		}
		if space == "oklab" || space == "oklch" {
			a = linearToOKLab(a)
			b = linearToOKLab(b)
		}
		if space == "oklch" {
			a = labToLCH(a)
			b = labToLCH(b)
		}
	}
	hueDelta := b[2] - a[2]
	if space == "oklch" {
		switch hue {
		case "shorter":
			if hueDelta > 180 {
				hueDelta -= 360
			}
			if hueDelta < -180 {
				hueDelta += 360
			}
		case "longer":
			if hueDelta >= 0 && hueDelta <= 180 {
				hueDelta -= 360
			} else if hueDelta < 0 && hueDelta >= -180 {
				hueDelta += 360
			}
		case "increasing":
			if hueDelta < 0 {
				hueDelta += 360
			}
		case "decreasing":
			if hueDelta >= 0 {
				hueDelta -= 360
			}
		}
	}
	return func(t float64) color.RGBA {
		alpha := g.From.A*(1-t) + g.To.A*t
		var mixed [3]float64
		for i := range 3 {
			mixed[i] = a[i]*g.From.A*(1-t) + b[i]*g.To.A*t
		}
		if space != "srgb" {
			if alpha > 0 {
				for i := range 3 {
					mixed[i] /= alpha
				}
			}
			if space == "oklch" {
				mixed[2] = a[2] + hueDelta*t
				mixed = lchToLab(mixed)
			}
			if space == "oklab" || space == "oklch" {
				mixed = okLabToLinear(mixed)
			}
			for i := range 3 {
				mixed[i] = math.Pow(min(max(mixed[i], 0), 1), 1/2.2) * alpha
			}
		}
		byteValue := func(v float64) uint8 { return uint8(math.Round(min(max(v, 0), 1) * 255)) }
		return color.RGBA{R: byteValue(mixed[0]), G: byteValue(mixed[1]), B: byteValue(mixed[2]), A: byteValue(alpha)}
	}
}

// These matrices and gamma 2.2 match Niri 26.04's border.frag.
func linearToOKLab(c [3]float64) [3]float64 {
	l := math.Cbrt(.4122214708*c[0] + .5363325363*c[1] + .0514459929*c[2])
	m := math.Cbrt(.2119034982*c[0] + .6806995451*c[1] + .1073969566*c[2])
	s := math.Cbrt(.0883024619*c[0] + .2817188376*c[1] + .6299787005*c[2])
	return [3]float64{.2104542553*l + .793617785*m - .0040720468*s, 1.9779984951*l - 2.428592205*m + .4505937099*s, .0259040371*l + .7827717662*m - .808675766*s}
}
func okLabToLinear(c [3]float64) [3]float64 {
	l := math.Pow(c[0]+.3963377774*c[1]+.2158037573*c[2], 3)
	m := math.Pow(c[0]-.1055613458*c[1]-.0638541728*c[2], 3)
	s := math.Pow(c[0]-.0894841775*c[1]-1.291485548*c[2], 3)
	return [3]float64{4.0767416621*l - 3.3077115913*m + .2309699292*s, -1.2684380046*l + 2.6097574011*m - .3413193965*s, -.0041960863*l - .7034186147*m + 1.707614701*s}
}
func labToLCH(c [3]float64) [3]float64 {
	hue := math.Atan2(c[2], c[1]) * 180 / math.Pi
	if hue <= 0 {
		hue += 360
	}
	return [3]float64{c[0], math.Hypot(c[1], c[2]), hue}
}
func lchToLab(c [3]float64) [3]float64 {
	return [3]float64{c[0], c[1] * math.Cos(c[2]*math.Pi/180), c[1] * math.Sin(c[2]*math.Pi/180)}
}
