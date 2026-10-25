package ui

import "testing"

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{"empty", "", 0},
		{"plain ascii", "hello", 5},
		{"plain ascii with spaces", "disk forensics", 14},
		{"ansi colored", "\033[32mhello\033[0m", 5},
		{"ansi colored with params", "\033[1;31mhello\033[0m", 5},
		{"east asian wide", "日本語", 6},
		{"mixed narrow and wide", "A日B", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayWidth(tt.s); got != tt.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

func TestDisplayWidthColorizedMatchesPlain(t *testing.T) {
	plain := "volatility"
	colored := "\033[32m" + plain + "\033[0m"
	if got, want := displayWidth(colored), displayWidth(plain); got != want {
		t.Errorf("colored width = %d, plain width = %d", got, want)
	}
}
