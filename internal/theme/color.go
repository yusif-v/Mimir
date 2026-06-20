package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ResolveColor converts a Color to an ANSI escape sequence.
// Supports: standard names ("green", "blue"), bright names ("bright_green"),
// 256-color ("38;5;214m"), direct codes ("38;5;214m"), and empty (no color).
func ResolveColor(c Color) string {
	if c == "" || os.Getenv("NO_COLOR") != "" {
		return ""
	}
	s := string(c)
	// If it already looks like a direct ANSI code (starts with "38;" or is a
	// plain number), wrap it.
	if strings.Contains(s, ";") || strings.HasPrefix(s, "\033[") {
		if !strings.HasSuffix(s, "m") {
			return "\033[" + s
		}
		return s
	}
	// Map named colors to ANSI codes.
	switch strings.ToLower(s) {
	case "black":
		return "\033[30m"
	case "red":
		return "\033[31m"
	case "green":
		return "\033[32m"
	case "yellow":
		return "\033[33m"
	case "blue":
		return "\033[34m"
	case "magenta":
		return "\033[35m"
	case "cyan":
		return "\033[36m"
	case "white":
		return "\033[37m"
	case "bright_black", "gray":
		return "\033[90m"
	case "bright_red":
		return "\033[91m"
	case "bright_green":
		return "\033[92m"
	case "bright_yellow":
		return "\033[93m"
	case "bright_blue":
		return "\033[94m"
	case "bright_magenta":
		return "\033[95m"
	case "bright_cyan":
		return "\033[96m"
	case "bright_white":
		return "\033[97m"
	case "dim":
		return "\033[2m"
	case "bold":
		return "\033[1m"
	case "italic":
		return "\033[3m"
	case "underline":
		return "\033[4m"
	default:
		// Try to parse as a raw number (e.g. "1" for bold).
		if code, err := strconv.Atoi(s); err == nil && code >= 0 && code <= 107 {
			return fmt.Sprintf("\033[%dm", code)
		}
		return ""
	}
}

// Colorize wraps s in the resolved color and a reset, unless NO_COLOR is set
// or the color is empty.
func Colorize(s string, c Color) string {
	code := ResolveColor(c)
	if code == "" {
		return s
	}
	return code + s + "\033[0m"
}
