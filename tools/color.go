package tools

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

var ColorInfo = &ToolDef{
	Name:        "color_info",
	Description: "Convert and get info about colors. Accepts hex (#ff5733), RGB (255,87,51), or HSL (9,100%,60%). Returns all formats, approximate color name, and complementary color.",
	Args: []ToolArg{
		{Name: "color", Description: "Color in any format: hex (#ff5733 or ff5733), RGB (255,87,51), or HSL (9,100%,60%)", Required: true},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["color"])
		if input == "" {
			return "Error: color is required"
		}

		r, g, b, err := parseColor(input)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		hex := fmt.Sprintf("#%02X%02X%02X", r, g, b)
		h, s, l := rgbToHsl(r, g, b)

		compH := (h + 180) % 360
		cr, cg, cb := hslToRgb(float64(compH), float64(s), float64(l))
		compHex := fmt.Sprintf("#%02X%02X%02X", cr, cg, cb)
		lum := relativeLuminance(r, g, b)
		contrast := "dark (use light text)"
		if lum < 0.5 {
			contrast = "dark (use light text)"
		} else {
			contrast = "light (use dark text)"
		}

		name := closestColorName(r, g, b)

		return fmt.Sprintf(
			"Color: %s\nName: ~%s\nHex: %s\nRGB: rgb(%d, %d, %d)\nLuminance: %.3f (%s)\nComplementary: %s (rgb(%d,%d,%d))",
			input, name, hex, r, g, b, lum, contrast, compHex, cr, cg, cb,
		)
	},
}

func parseColor(s string) (r, g, b uint8, err error) {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "#") || isHexColor(s) {
		hex := strings.TrimPrefix(s, "#")
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return 0, 0, 0, fmt.Errorf("invalid hex color %q", s)
		}
		rv, e1 := strconv.ParseUint(hex[0:2], 16, 8)
		gv, e2 := strconv.ParseUint(hex[2:4], 16, 8)
		bv, e3 := strconv.ParseUint(hex[4:6], 16, 8)
		if e1 != nil || e2 != nil || e3 != nil {
			return 0, 0, 0, fmt.Errorf("invalid hex color %q", s)
		}
		return uint8(rv), uint8(gv), uint8(bv), nil
	}

	parts := strings.FieldsFunc(s, func(c rune) bool { return c == ',' || c == ' ' || c == '(' || c == ')' })

	if len(parts) > 0 && strings.EqualFold(parts[0], "rgb") {
		parts = parts[1:]
	}

	if len(parts) == 3 {

		if strings.Contains(s, "%") {

			hv, e1 := strconv.ParseFloat(strings.TrimSuffix(parts[0], "%"), 64)
			sv, e2 := strconv.ParseFloat(strings.TrimSuffix(parts[1], "%"), 64)
			lv, e3 := strconv.ParseFloat(strings.TrimSuffix(parts[2], "%"), 64)
			if e1 != nil || e2 != nil || e3 != nil {
				return 0, 0, 0, fmt.Errorf("invalid HSL values")
			}
			rr, gg, bb := hslToRgb(hv, sv, lv)
			return rr, gg, bb, nil
		}

		rv, e1 := strconv.ParseFloat(parts[0], 64)
		gv, e2 := strconv.ParseFloat(parts[1], 64)
		bv, e3 := strconv.ParseFloat(parts[2], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			return 0, 0, 0, fmt.Errorf("invalid RGB values")
		}
		return uint8(clamp(rv, 0, 255)), uint8(clamp(gv, 0, 255)), uint8(clamp(bv, 0, 255)), nil
	}

	return 0, 0, 0, fmt.Errorf("unrecognized color format %q — use hex (#ff5733), RGB (255,87,51), or HSL (9,100%%,60%%)", s)
}

func isHexColor(s string) bool {
	if len(s) != 3 && len(s) != 6 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func rgbToHsl(r, g, b uint8) (h, s, l int) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	delta := maxC - minC
	lf := (maxC + minC) / 2

	var hf, sf float64
	if delta != 0 {
		if lf < 0.5 {
			sf = delta / (maxC + minC)
		} else {
			sf = delta / (2 - maxC - minC)
		}
		switch maxC {
		case rf:
			hf = (gf-bf)/delta + 0
			if gf < bf {
				hf += 6
			}
		case gf:
			hf = (bf-rf)/delta + 2
		case bf:
			hf = (rf-gf)/delta + 4
		}
		hf *= 60
	}

	return int(math.Round(hf)), int(math.Round(sf * 100)), int(math.Round(lf * 100))
}

func hslToRgb(h, s, l float64) (uint8, uint8, uint8) {
	s /= 100
	l /= 100
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	rf := hueToRgb(p, q, h/360+1.0/3)
	gf := hueToRgb(p, q, h/360)
	bf := hueToRgb(p, q, h/360-1.0/3)
	return uint8(math.Round(rf * 255)), uint8(math.Round(gf * 255)), uint8(math.Round(bf * 255))
}

func hueToRgb(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

func relativeLuminance(r, g, b uint8) float64 {
	toLinear := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}

var namedColors = []struct {
	name    string
	r, g, b uint8
}{
	{"red", 255, 0, 0}, {"green", 0, 128, 0}, {"blue", 0, 0, 255},
	{"yellow", 255, 255, 0}, {"cyan", 0, 255, 255}, {"magenta", 255, 0, 255},
	{"white", 255, 255, 255}, {"black", 0, 0, 0}, {"gray", 128, 128, 128},
	{"orange", 255, 165, 0}, {"pink", 255, 192, 203}, {"purple", 128, 0, 128},
	{"brown", 165, 42, 42}, {"lime", 0, 255, 0}, {"navy", 0, 0, 128},
	{"teal", 0, 128, 128}, {"maroon", 128, 0, 0}, {"olive", 128, 128, 0},
	{"coral", 255, 127, 80}, {"salmon", 250, 128, 114}, {"gold", 255, 215, 0},
	{"violet", 238, 130, 238}, {"indigo", 75, 0, 130}, {"crimson", 220, 20, 60},
	{"turquoise", 64, 224, 208}, {"silver", 192, 192, 192}, {"khaki", 240, 230, 140},
	{"beige", 245, 245, 220}, {"lavender", 230, 230, 250}, {"mint", 62, 180, 137},
}

func closestColorName(r, g, b uint8) string {
	best := ""
	bestDist := math.MaxFloat64
	for _, c := range namedColors {
		dr := float64(r) - float64(c.r)
		dg := float64(g) - float64(c.g)
		db := float64(b) - float64(c.b)
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			best = c.name
		}
	}
	return best
}
