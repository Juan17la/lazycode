package app

import "github.com/gdamore/tcell/v2"

// theme holds the colours in use (Q-48): one primary and one accent, on the terminal's own
// background. Bold, dim, reverse and underline carry the meaning too, so nothing depends on
// colour. tcell picks the closest colour that a low-colour terminal has.
type theme struct {
	primary, accent, muted tcell.Style
	active                 tcell.Style // Whatever has focus, and selections.
	otherCursor            tcell.Style // Cursors the terminal can't draw itself.
	match                  tcell.Style // Find matches on screen.
	err                    tcell.Style
}

// Each accent and muted colour was picked for at least 4.5:1 contrast on that theme's usual
// background: 208 on black is about 9:1, and 130 on white about 4.6:1.
func themeFor(name string) theme {
	accent, muted, red := tcell.PaletteColor(208), tcell.PaletteColor(245), tcell.PaletteColor(203)
	if name == "light" {
		accent, muted, red = tcell.PaletteColor(130), tcell.PaletteColor(240), tcell.PaletteColor(160)
	}
	plain := tcell.StyleDefault
	return theme{
		primary:     plain,
		accent:      plain.Foreground(accent).Bold(true),
		muted:       plain.Foreground(muted),
		active:      plain.Reverse(true),
		otherCursor: plain.Foreground(accent).Reverse(true),
		match:       plain.Foreground(accent).Underline(true),
		err:         plain.Foreground(red).Bold(true),
	}
}
