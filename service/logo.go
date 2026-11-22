package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// logoArt is assets/logo/icon.svg redrawn by hand at 16x16; a scaled-down PNG blurs the mouth away.
// . transparent, P background gradient, W white, D mouth, T tongue.
var logoArt = []string{
	".PPPPPPPPPPPPPP.",
	"PPPPPPPPPPPPPPPP",
	"PPWWWPPPPPPWWWPP",
	"PPWPPPPPPPPPPWPP",
	"PPWPPPPPPPPPPWPP",
	"PPPPPPPPPPPPPPPP",
	"PPPWWWWWWWWWWPPP",
	"PPPWDWWWWWWDWPPP",
	"PPPWDDDDDDDDWPPP",
	"PPPPWDTTTTDWPPPP",
	"PPPPPWWWWWWPPPPP",
	"PPWPPPPPPPPPPWPP",
	"PPWPPPPPPPPPPWPP",
	"PPWWWPPPPPPWWWPP",
	"PPPPPPPPPPPPPPPP",
	".PPPPPPPPPPPPPP.",
}

// logoLines renders logoArt with half blocks: each text row shows two pixel rows.
func logoLines() []string {
	gradient := lipgloss.Blend1D(31, cPurple, cPink) // indexed by x+y, the icon's diagonal
	pixel := func(x, y int) color.Color {
		switch logoArt[y][x] {
		case 'W':
			return lipgloss.Color("#ffffff")
		case 'D':
			return lipgloss.Color("#2e1065")
		case 'T':
			return lipgloss.Color("#fb7185")
		case 'P':
			return gradient[x+y]
		}
		return nil
	}
	var lines []string
	for y := 0; y < len(logoArt); y += 2 {
		var b strings.Builder
		for x := range logoArt[y] {
			top, bottom := pixel(x, y), pixel(x, y+1)
			switch {
			case top == nil && bottom == nil:
				b.WriteString(" ")
			case top == nil:
				b.WriteString(lipgloss.NewStyle().Foreground(bottom).Render("▄"))
			case bottom == nil:
				b.WriteString(lipgloss.NewStyle().Foreground(top).Render("▀"))
			default:
				b.WriteString(lipgloss.NewStyle().Foreground(top).Background(bottom).Render("▀"))
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}
