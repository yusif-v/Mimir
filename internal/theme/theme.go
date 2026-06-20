// Package theme provides preset themes and color customization for the Mimir
// prompt and UI. Themes are stored as YAML under themes/ and loaded at startup.
package theme

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Color is a single ANSI color slot. Values are raw ANSI escape names
// ("green", "bright_blue") or direct codes (e.g. "38;5;214m"). The shell
// runtime resolves them to escape sequences.
type Color string

// PromptStyle controls the layout of the status line.
type PromptStyle string

const (
	PromptStyleInline  PromptStyle = "inline"  // [user] Mimir [case] [status] ❯
	PromptStyleTwoLine PromptStyle = "twoline" // status line \n ❯
)

// Theme is a complete visual theme definition.
type Theme struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Prompt      PromptStyle  `yaml:"prompt"`
	Layout      Layout      `yaml:"layout"`
	Colors      ColorMap    `yaml:"colors"`
	Segments    SegmentList `yaml:"segments"`
}

// Layout controls spacing and borders of the prompt.
type Layout struct {
	// Separator is the string between prompt segments (default: " ").
	Separator string `yaml:"separator"`
	// LeftPad / RightPad spaces around the prompt marker.
	LeftPad  string `yaml:"left_pad"`
	RightPad string `yaml:"right_pad"`
	// Marker is the prompt input marker (default: "❯"). Use "$" for ASCII.
	Marker string `yaml:"marker"`
}

// ColorMap holds the named color slots used by the prompt.
type ColorMap struct {
	User     Color `yaml:"user"`
	Brand    Color `yaml:"brand"`
	Case     Color `yaml:"case"`
	Status   Color `yaml:"status"`
	StatusOK Color `yaml:"status_ok"`
	StatusErr Color `yaml:"status_err"`
	Border   Color `yaml:"border"`
	Dim      Color `yaml:"dim"`
}

// SegmentList is the ordered segments shown in the prompt line.
type SegmentList []Segment

// Segment is one piece of the status line.
type Segment struct {
	Type  string `yaml:"type"` // "user" | "brand" | "case" | "status" | "custom"
	Text  string `yaml:"text"` // only used for type=custom
	Color Color  `yaml:"color"` // override color (optional)
}

// DefaultTheme returns the built-in default theme.
func DefaultTheme() Theme {
	return Theme{
		Name:        "default",
		Description: "Mimir default prompt — blue brand, green user, yellow case",
		Prompt:      PromptStyleInline,
		Layout: Layout{
			Separator: " ",
			LeftPad:  "",
			RightPad: " ",
			Marker:   "❯",
		},
		Colors: ColorMap{
			User:     "green",
			Brand:    "blue",
			Case:     "yellow",
			Status:   "dim",
			StatusOK: "green",
			StatusErr: "red",
			Border:   "dim",
			Dim:      "dim",
		},
		Segments: SegmentList{
			{Type: "user"},
			{Type: "brand", Text: "Mimir"},
			{Type: "case"},
			{Type: "status"},
		},
	}
}

// KaliTheme returns a Kali Linux-inspired dark theme.
func KaliTheme() Theme {
	return Theme{
		Name:        "kali",
		Description: "Kali Linux style — dark background, cyan/blue accents, box-drawing borders",
		Prompt:      PromptStyleTwoLine,
		Layout: Layout{
			Separator: " ",
			LeftPad:  " ",
			RightPad: " ",
			Marker:   "❯",
		},
		Colors: ColorMap{
			User:     "cyan",
			Brand:    "bright_cyan",
			Case:     "white",
			Status:   "dim",
			StatusOK: "green",
			StatusErr: "red",
			Border:   "cyan",
			Dim:      "dim",
		},
		Segments: SegmentList{
			{Type: "user"},
			{Type: "brand", Text: "Mimir"},
			{Type: "case"},
			{Type: "status"},
		},
	}
}

// MinimalTheme returns a stripped-down monochrome theme.
func MinimalTheme() Theme {
	return Theme{
		Name:        "minimal",
		Description: "Minimal — no colors, ASCII markers, clean layout",
		Prompt:      PromptStyleInline,
		Layout: Layout{
			Separator: " ",
			LeftPad:  "",
			RightPad: " ",
			Marker:   ">",
		},
		Colors: ColorMap{
			User:     "white",
			Brand:    "white",
			Case:     "white",
			Status:   "white",
			StatusOK: "white",
			StatusErr: "white",
			Border:   "white",
			Dim:      "white",
		},
		Segments: SegmentList{
			{Type: "user"},
			{Type: "brand", Text: "Mimir"},
			{Type: "case"},
			{Type: "status"},
		},
	}
}

// NoColorTheme returns a theme that emits no ANSI color codes at all.
func NoColorTheme() Theme {
	return Theme{
		Name:        "no-color",
		Description: "No ANSI colors — pure text output, respects NO_COLOR",
		Prompt:      PromptStyleInline,
		Layout: Layout{
			Separator: " ",
			LeftPad:  "",
			RightPad: " ",
			Marker:   "❯",
		},
		Colors: ColorMap{
			User:     "",
			Brand:    "",
			Case:     "",
			Status:   "",
			StatusOK: "",
			StatusErr: "",
			Border:   "",
			Dim:      "",
		},
		Segments: SegmentList{
			{Type: "user"},
			{Type: "brand", Text: "Mimir"},
			{Type: "case"},
			{Type: "status"},
		},
	}
}

// BuiltinThemes returns all built-in themes.
func BuiltinThemes() []Theme {
	return []Theme{
		DefaultTheme(),
		KaliTheme(),
		MinimalTheme(),
		NoColorTheme(),
	}
}

// Load reads a theme from a YAML file. If the file doesn't exist, the default
// theme is returned.
func Load(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultTheme(), fmt.Errorf("theme: read %s: %w", path, err)
	}
	var t Theme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return DefaultTheme(), fmt.Errorf("theme: parse %s: %w", path, err)
	}
	return t, nil
}

// Save writes a theme to a YAML file.
func Save(t Theme, path string) error {
	data, err := yaml.Marshal(t)
	if err != nil {
		return fmt.Errorf("theme: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// BuiltinTheme returns a built-in theme by name, or default if not found.
func BuiltinTheme(name string) Theme {
	for _, t := range BuiltinThemes() {
		if t.Name == name {
			return t
		}
	}
	return DefaultTheme()
}
