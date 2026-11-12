package app

import (
	"github.com/Juan17la/lazycode/internal/config"
	"github.com/gdamore/tcell/v2"
)

// palette is a theme's colours, as #rrggbb. Its keys are config.ColorKeys, so that [ui.colors]
// can change any of them.
type palette map[string]string

func pal(bg, panel, bar, selection, fg, muted, comment, accent, red, green, yellow, blue, purple, cyan, orange string) palette {
	return palette{"bg": bg, "panel": panel, "bar": bar, "selection": selection, "fg": fg, "muted": muted, "comment": comment, "accent": accent,
		"red": red, "green": green, "yellow": yellow, "blue": blue, "purple": purple, "cyan": cyan, "orange": orange}
}

// palettes holds every name in config.Themes, a test checks. Values follow each theme's published
// colours, with some muted and comment colours lightened or darkened until they're readable.
var palettes = map[string]palette{
	"dark":             pal("#1e1e1e", "#252526", "#181818", "#264f78", "#d4d4d4", "#909090", "#6a9955", "#569cd6", "#f48771", "#89d185", "#dcdcaa", "#569cd6", "#c586c0", "#4ec9b0", "#ce9178"),
	"light":            pal("#ffffff", "#f3f3f3", "#e8e8e8", "#add6ff", "#1f1f1f", "#6a6a6a", "#008000", "#0066b8", "#c72e0f", "#2e7d32", "#795e26", "#0451a5", "#af00db", "#267f99", "#a31515"),
	"gruvbox-dark":     pal("#282828", "#32302f", "#1d2021", "#504945", "#ebdbb2", "#a89984", "#928374", "#fe8019", "#fb4934", "#b8bb26", "#fabd2f", "#83a598", "#d3869b", "#8ec07c", "#fe8019"),
	"gruvbox-light":    pal("#fbf1c7", "#f2e5bc", "#ebdbb2", "#d5c4a1", "#3c3836", "#665c54", "#7c6f64", "#af3a03", "#9d0006", "#79740e", "#b57614", "#076678", "#8f3f71", "#427b58", "#af3a03"),
	"dracula":          pal("#282a36", "#21222c", "#191a21", "#44475a", "#f8f8f2", "#a4aed0", "#7f8cb8", "#bd93f9", "#ff5555", "#50fa7b", "#f1fa8c", "#8be9fd", "#ff79c6", "#8be9fd", "#ffb86c"),
	"one-dark":         pal("#282c34", "#21252b", "#1b1f23", "#3e4451", "#abb2bf", "#8b92a0", "#7f848e", "#61afef", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#d19a66"),
	"nord":             pal("#2e3440", "#3b4252", "#272c36", "#4c566a", "#d8dee9", "#a3adbf", "#8f9bb3", "#88c0d0", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#8fbcbb", "#d08770"),
	"tokyo-night":      pal("#1a1b26", "#16161e", "#13131a", "#33467c", "#c0caf5", "#8089b3", "#6f7bb0", "#7aa2f7", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#ff9e64"),
	"catppuccin-mocha": pal("#1e1e2e", "#181825", "#11111b", "#45475a", "#cdd6f4", "#a6adc8", "#7f849c", "#cba6f7", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#cba6f7", "#94e2d5", "#fab387"),
	"solarized-dark":   pal("#002b36", "#073642", "#00232c", "#0f4b5a", "#93a1a1", "#839496", "#7d9299", "#2aa198", "#ef5350", "#9aab00", "#c79a00", "#4a9fe0", "#8c8fd8", "#2aa198", "#e0703a"),
	"solarized-light":  pal("#fdf6e3", "#eee8d5", "#e4dcc5", "#d6cfb8", "#44555c", "#586e75", "#6f8087", "#1a7f78", "#c02a28", "#6b7a00", "#8f6d00", "#1f6fb0", "#5a5cb0", "#1a7f78", "#b0420f"),
	"monokai":          pal("#272822", "#1e1f1c", "#191a16", "#49483e", "#f8f8f2", "#a8a692", "#8f8c78", "#a6e22e", "#f92672", "#a6e22e", "#e6db74", "#66d9ef", "#ae81ff", "#66d9ef", "#fd971f"),
}

// theme holds the styles in use. They keep the default background: paintBackground fills it in
// by region, so that no drawing code has to know which region it's in.
type theme struct {
	bg, panel, bar         tcell.Color
	fg                     tcell.Color
	primary, accent, muted tcell.Style
	active                 tcell.Style // Whatever has focus, and selections.
	otherCursor            tcell.Style // Cursors the terminal can't draw itself.
	match                  tcell.Style // Find matches on screen.
	success                tcell.Style // A session wanting attention (Q-36).
	err                    tcell.Style
	tabCurrent             tcell.Style
	border, borderFocus    tcell.Style // Lines between areas, and those around what has focus.
	// Syntax. Keywords stay bold and comments italic, so meaning survives without colour.
	keyword, str, number, constant, function, typ, variable, property, operator, tag, comment tcell.Style
	// Git state in the tree, each with a letter too (Q-48).
	gitAdded, gitModified, gitUntracked, gitConflict tcell.Style
	icon                                             map[string]tcell.Style // By palette colour name.
}

// plainText takes out the bold and italics, for fonts that draw them badly.
func (th *theme) plainText() {
	for _, s := range []*tcell.Style{&th.accent, &th.err, &th.success, &th.tabCurrent, &th.borderFocus, &th.keyword, &th.comment, &th.gitConflict} {
		*s = s.Bold(false).Italic(false)
	}
}

func themeFor(name string) theme { return themeWith(name, nil) }

// themeWith is a named theme with the user's [ui.colors] over it.
func themeWith(name string, custom map[string]string) theme {
	p, ok := palettes[name]
	if !ok {
		p = palettes[config.DefaultTheme]
	}
	c := func(key string) tcell.Color {
		if v, ok := custom[key]; ok {
			return tcell.GetColor(v)
		}
		return tcell.GetColor(p[key])
	}
	plain := tcell.StyleDefault
	fg := func(key string) tcell.Style { return plain.Foreground(c(key)) }
	th := theme{
		bg: c("bg"), panel: c("panel"), bar: c("bar"), fg: c("fg"),
		primary:      plain,
		accent:       fg("accent").Bold(true),
		muted:        fg("muted"),
		active:       plain.Background(c("selection")).Foreground(c("fg")),
		otherCursor:  fg("accent").Reverse(true),
		match:        fg("yellow").Underline(true),
		err:          fg("red").Bold(true),
		success:      fg("green").Bold(true),
		border:       fg("blue"),
		borderFocus:  fg("accent").Bold(true),
		tabCurrent:   plain.Background(c("bg")).Foreground(c("fg")).Bold(true),
		keyword:      fg("purple").Bold(true),
		str:          fg("green"),
		number:       fg("orange"),
		constant:     fg("orange"),
		function:     fg("blue"),
		typ:          fg("yellow"),
		variable:     fg("red"),
		property:     fg("cyan"),
		operator:     fg("cyan"),
		tag:          fg("red"),
		comment:      fg("comment").Italic(true),
		gitAdded:     fg("green"),
		gitModified:  fg("yellow"),
		gitUntracked: fg("cyan"),
		gitConflict:  fg("red").Bold(true),
		icon:         map[string]tcell.Style{},
	}
	for _, key := range []string{"muted", "accent", "red", "green", "yellow", "blue", "purple", "cyan", "orange"} {
		th.icon[key] = fg(key)
	}
	return th
}

// pill is a coloured label: the theme's colour behind its background colour. With few colours,
// it's reversed, which is the part that always shows.
func (a *app) pill(colour string) tcell.Style {
	fg, _, _ := a.th.icon[colour].Decompose()
	if a.lowColour() {
		return tcell.StyleDefault.Reverse(true).Bold(true)
	}
	return tcell.StyleDefault.Background(fg).Foreground(a.th.bg).Bold(true)
}

// setTheme applies a theme. A terminal with few colours keeps its own background, and selections
// are reversed there, because a subtle shade could round to the same colour as the text behind it.
func (a *app) setTheme(name string) {
	a.th = themeWith(name, a.cfg.UI.Colors)
	if a.cfg.UI.TextStyle == "plain" {
		a.th.plainText()
	}
	if a.lowColour() {
		a.th.active = tcell.StyleDefault.Reverse(true)
		a.th.tabCurrent = tcell.StyleDefault.Bold(true).Underline(true)
	}
}

func (a *app) lowColour() bool { return a.screen != nil && a.screen.Colors() < 256 }

// paintBackground gives every cell still on the default colours its region's. It runs last, so it
// also covers terminal tabs, which draw default-coloured cells of their own.
func (a *app) paintBackground(st *dirState, w, h int) {
	if a.lowColour() {
		return
	}
	first, second, _ := a.regions()
	var panes []*pane
	if !a.aiMode { // Their areas are from the editor's last frame.
		panes = a.shownPanes(st)
	}
	for y := range h {
		for x := 0; x < w; x++ {
			text, style, width := a.screen.Get(x, y)
			fg, bg, _ := style.Decompose()
			if fg == tcell.ColorDefault || bg == tcell.ColorDefault {
				region := a.th.bg
				switch {
				case y == 0 || y == h-1:
					region = a.th.bar
				case first.contains(x, y) || second.contains(x, y):
					region = a.th.panel
				default:
					for _, p := range panes {
						if y == p.area.y && x >= p.area.x && x < p.area.x+p.area.w {
							region = a.th.panel
						}
					}
				}
				for _, o := range a.floating {
					if o.contains(x, y) {
						region = a.th.bar
					}
				}
				if fg == tcell.ColorDefault {
					style = style.Foreground(a.th.fg)
				}
				if bg == tcell.ColorDefault {
					style = style.Background(region)
				}
				if text == "" {
					text = " "
				}
				a.screen.Put(x, y, text, style)
			}
			if width > 1 {
				x += width - 1 // The second cell belongs to this character.
			}
		}
	}
}

// blend mixes percent of over into base.
func blend(base, over tcell.Color, percent int32) tcell.Color {
	r1, g1, b1 := base.RGB()
	r2, g2, b2 := over.RGB()
	mix := func(a, b int32) int32 { return a + (b-a)*percent/100 }
	return tcell.NewRGBColor(mix(r1, r2), mix(g1, g2), mix(b1, b2))
}

func bgOf(s tcell.Style) tcell.Color {
	_, bg, _ := s.Decompose()
	return bg
}
