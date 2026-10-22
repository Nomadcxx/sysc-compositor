package text

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"
)

// Span is a run of text in one style.
type Span struct {
	Text  string
	Style Style
}

// Style is what a markup tag can set on a span. The zero value draws in the
// layout's font and colour with no background.
type Style struct {
	Family string // empty: the layout's family
	Bold   bool
	Italic bool
	// FG and BG are nil when unset: FG then takes the layout's colour, and
	// no background is drawn.
	FG, BG *color.NRGBA
	// LetterSpacing is in Pango units, 1/1024 of a point, as in markup.
	LetterSpacing int
}

// Plain is s as one unstyled span.
func Plain(s string) []Span { return []Span{{Text: s}} }

// ParseMarkup parses the subset of Pango markup that Niri's UIs and
// hotkey-overlay titles use: <span> with face/font_family, fgcolor/foreground/
// color, bgcolor/background, letter_spacing, weight and style; <b>, <i> and
// <tt>; and XML entities. Anything else is an error, so a caller can fall back
// to the plain text as Niri does.
func ParseMarkup(s string) ([]Span, error) {
	dec := xml.NewDecoder(strings.NewReader("<markup>" + s + "</markup>"))
	dec.Strict = true
	stack := []Style{{}}
	var spans []Span
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("text: markup: %w", err)
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "markup" && len(stack) == 1 {
				continue
			}
			style, err := applyTag(top, t)
			if err != nil {
				return nil, err
			}
			stack = append(stack, style)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if n := len(spans); n > 0 && spans[n-1].Style == top {
				spans[n-1].Text += string(t)
			} else if len(t) > 0 {
				spans = append(spans, Span{Text: string(t), Style: top})
			}
		}
	}
	return spans, nil
}

// PlainText is the spans' text without styling.
func PlainText(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func applyTag(style Style, t xml.StartElement) (Style, error) {
	switch t.Name.Local {
	case "b":
		style.Bold = true
	case "i":
		style.Italic = true
	case "tt":
		style.Family = "monospace"
	case "span":
		for _, a := range t.Attr {
			if err := applyAttr(&style, a.Name.Local, a.Value); err != nil {
				return style, err
			}
		}
	default:
		return style, fmt.Errorf("text: markup: unsupported tag <%s>", t.Name.Local)
	}
	return style, nil
}

func applyAttr(style *Style, name, value string) error {
	switch name {
	case "face", "font_family":
		style.Family = value
	case "fgcolor", "foreground", "color":
		c, err := parseColor(value)
		if err != nil {
			return err
		}
		style.FG = &c
	case "bgcolor", "background":
		c, err := parseColor(value)
		if err != nil {
			return err
		}
		style.BG = &c
	case "letter_spacing":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("text: markup: letter_spacing %q", value)
		}
		style.LetterSpacing = n
	case "weight":
		switch value {
		case "bold", "ultrabold", "heavy":
			style.Bold = true
		case "normal":
			style.Bold = false
		default:
			return fmt.Errorf("text: markup: weight %q", value)
		}
	case "style":
		switch value {
		case "italic", "oblique":
			style.Italic = true
		case "normal":
			style.Italic = false
		default:
			return fmt.Errorf("text: markup: style %q", value)
		}
	default:
		return fmt.Errorf("text: markup: unsupported span attribute %q", name)
	}
	return nil
}

// parseColor takes #rgb, #rrggbb or #rrggbbaa.
func parseColor(s string) (color.NRGBA, error) {
	hex, ok := strings.CutPrefix(s, "#")
	if ok && len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if !ok || (len(hex) != 6 && len(hex) != 8) {
		return color.NRGBA{}, fmt.Errorf("text: markup: colour %q", s)
	}
	if len(hex) == 6 {
		hex += "ff"
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("text: markup: colour %q", s)
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}
