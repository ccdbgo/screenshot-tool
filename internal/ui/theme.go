package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ElegantTheme overrides the primary/focus/selection colors with a deep-indigo
// blue palette, replacing Fyne's default teal-green accent.
type ElegantTheme struct{}

// NewElegantTheme returns a Fyne theme with an elegant blue-indigo accent.
func NewElegantTheme() fyne.Theme { return &ElegantTheme{} }

func (t *ElegantTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		// #1A56DB — deep indigo-blue: refined, not green
		return color.NRGBA{R: 26, G: 86, B: 219, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 26, G: 86, B: 219, A: 210}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 26, G: 86, B: 219, A: 72}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 26, G: 86, B: 219, A: 255}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *ElegantTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *ElegantTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *ElegantTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
