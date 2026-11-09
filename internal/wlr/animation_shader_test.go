package wlr

import (
	"image"
	"image/color"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestCustomAnimationShadersRenderPixelsAndRetainValidProgram(t *testing.T) {
	c := newNested(t)
	shader := `vec4 open_color(vec3 coords_geo, vec3 size_geo) { return vec4(niri_progress, texture2D(niri_tex, (niri_geo_to_tex * coords_geo).xy).g, niri_clamped_progress, 1.0); }`
	c.animationConfig.WindowOpen.CustomShader = &shader
	c.configureAnimationShaders()
	input := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			input.SetRGBA(x, y, color.RGBA{G: 255, A: 255})
		}
	}
	frame, err := c.renderAnimationShaderImage("open", input, input, .25, .75)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.RGBAAt(2, 2); got.R < 62 || got.R > 66 || got.G != 255 || got.B < 190 || got.B > 193 || got.A != 255 {
		t.Fatalf("shader output = %v", got)
	}
	invalid := "this is invalid GLSL"
	c.animationConfig.WindowOpen.CustomShader = &invalid
	c.configureAnimationShaders()
	frame, err = c.renderAnimationShaderImage("open", input, input, .25, .75)
	if err != nil || frame.RGBAAt(2, 2).G != 255 {
		t.Fatalf("invalid reload discarded the working program: %v", err)
	}
	for kind, source := range map[string]string{
		"close":  `vec4 close_color(vec3 c, vec3 s) { return texture2D(niri_tex, (niri_geo_to_tex*c).xy) * (1.0-niri_progress); }`,
		"resize": `vec4 resize_color(vec3 c, vec3 s) { return mix(texture2D(niri_tex_prev, (niri_geo_to_tex_prev*c).xy),texture2D(niri_tex_next, (niri_geo_to_tex_next*c).xy),niri_progress); }`,
	} {
		if kind == "close" {
			c.animationConfig.WindowClose.CustomShader = &source
		} else {
			c.animationConfig.WindowResize.CustomShader = &source
		}
		c.configureAnimationShaders()
		if _, err := c.renderAnimationShaderImage(kind, input, input, .5, .5); err != nil {
			t.Fatalf("%s shader: %v", kind, err)
		}
	}
	c.animationConfig = config.Default().Animations
	c.configureAnimationShaders()
	if _, err := c.renderAnimationShaderImage("open", input, input, 0, 0); err == nil {
		t.Fatal("shader removal retained the custom program")
	}
}
