package wlr

import (
	"image"
	"math"
	"slices"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

// Niri's guess_monitor_scale constants (src/utils/scale.rs).
const (
	scaleGuessMin            = 1
	scaleGuessMax            = 4
	scaleGuessSteps          = 4
	scaleGuessMinLogicalArea = 800 * 480
	scaleGuessMobileDPI      = 135.0
	scaleGuessLargeDPI       = 110.0
	scaleGuessLargeInches    = 20.0
)

// guessOutputScale is Niri's guess_monitor_scale: the quarter step from 1 to 4
// nearest the scale that brings the panel to a target density, among those
// that leave at least 800x480 logical pixels. An unknown size gives 1.
func guessOutputScale(widthMM, heightMM, width, height int) float64 {
	if widthMM <= 0 || heightMM <= 0 {
		return 1
	}
	diagonal := math.Hypot(float64(widthMM), float64(heightMM)) / 25.4
	target := scaleGuessMobileDPI
	if diagonal >= scaleGuessLargeInches {
		target = scaleGuessLargeDPI
	}
	perfect := math.Hypot(float64(width), float64(height)) / diagonal / target
	best, bestDistance := 1.0, math.Inf(1)
	for step := scaleGuessMin * scaleGuessSteps; step <= scaleGuessMax*scaleGuessSteps; step++ {
		scale := float64(step) / scaleGuessSteps
		logicalW := int(math.Round(float64(width) / scale))
		logicalH := int(math.Round(float64(height) / scale))
		if logicalW*logicalH < scaleGuessMinLogicalArea {
			continue
		}
		if distance := math.Abs(scale - perfect); distance < bestDistance {
			best, bestDistance = scale, distance
		}
	}
	return best
}

// outputScale is the scale Niri gives an output: the configured one or the
// guess, clamped to 0.1–10 and rounded to the N/120 steps that the
// fractional-scale protocol can express.
func outputScale(configured *float64, widthMM, heightMM, width, height int) float64 {
	scale := guessOutputScale(widthMM, heightMM, width, height)
	if configured != nil {
		scale = *configured
	}
	return math.Round(min(max(scale, 0.1), 10)*120) / 120
}

// outputModeChoice is one mode an output lists; refresh is in mHz.
type outputModeChoice struct {
	width, height, refresh int
	preferred              bool
}

// pickOutputMode is Niri's pick_mode over the modes an output lists. With a
// refresh rate, only a mode of that size whose refresh matches in mHz; without
// one, the fastest mode of that size. When the target matches nothing, or there
// is none, the fastest preferred mode, then the first. fallback reports that a
// target was given and missed. index is -1 when the output lists no mode.
func pickOutputMode(modes []outputModeChoice, target *config.OutputMode) (index int, fallback bool) {
	index = -1
	if target != nil {
		refresh := int(math.Round(target.Refresh * 1000))
		for i, mode := range modes {
			if mode.width != target.Width || mode.height != target.Height {
				continue
			}
			switch {
			case target.Refresh > 0:
				if mode.refresh == refresh {
					index = i
				}
			case index < 0 || modes[index].refresh < mode.refresh:
				index = i
			}
		}
		if index >= 0 {
			return index, false
		}
		fallback = true
	}
	for i, mode := range modes {
		if mode.preferred && (index < 0 || modes[index].refresh < mode.refresh) {
			index = i
		}
	}
	if index < 0 && len(modes) > 0 {
		index = 0
	}
	return index, fallback
}

// outputPlacement is one output to position: its identity, its logical size
// and its configured position, if any.
type outputPlacement struct {
	name       config.OutputName
	size       image.Point
	configured *config.OutputPosition
}

// placeOutputs is Niri's reposition_outputs. Outputs with a configured
// position go first and get it unless it overlaps an output already placed;
// the rest go right of everything placed so far, at y 0. Within each group
// the order is OutputName.Compare, so it does not depend on the order the
// outputs connected. The positions come back in the order of outputs.
func placeOutputs(outputs []outputPlacement) []image.Point {
	order := make([]int, len(outputs))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		aAuto, bAuto := outputs[a].configured == nil, outputs[b].configured == nil
		if aAuto != bAuto {
			if aAuto {
				return 1
			}
			return -1
		}
		return outputs[a].name.Compare(outputs[b].name)
	})
	positions := make([]image.Point, len(outputs))
	var placed []image.Rectangle
	for _, i := range order {
		output := outputs[i]
		position, ok := image.Point{}, false
		if output.configured != nil {
			position = image.Pt(output.configured.X, output.configured.Y)
			target := image.Rectangle{Min: position, Max: position.Add(output.size)}
			ok = !slices.ContainsFunc(placed, target.Overlaps)
		}
		if !ok {
			x := 0
			for _, box := range placed {
				x = max(x, box.Max.X)
			}
			position = image.Pt(x, 0)
		}
		positions[i] = position
		placed = append(placed, image.Rectangle{Min: position, Max: position.Add(output.size)})
	}
	return positions
}
