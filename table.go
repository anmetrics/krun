package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Table struct {
	Headers []string
	Rows    [][]string
}

func (t *Table) Render() string {
	if len(t.Headers) == 0 {
		return ""
	}

	cols := len(t.Headers)
	widths := make([]int, cols)

	for i, h := range t.Headers {
		widths[i] = visibleLen(h)
	}
	for _, row := range t.Rows {
		for i := 0; i < cols && i < len(row); i++ {
			l := visibleLen(row[i])
			if l > widths[i] {
				widths[i] = l
			}
		}
	}

	var b strings.Builder

	// Top border
	b.WriteString("┌")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < cols-1 {
			b.WriteString("┬")
		}
	}
	b.WriteString("┐\n")

	// Header row
	b.WriteString("│")
	for i, h := range t.Headers {
		b.WriteString(" ")
		b.WriteString(colorize(bold, padRight(h, widths[i])))
		b.WriteString(" │")
	}
	b.WriteString("\n")

	// Header separator
	b.WriteString("├")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < cols-1 {
			b.WriteString("┼")
		}
	}
	b.WriteString("┤\n")

	// Data rows
	for _, row := range t.Rows {
		b.WriteString("│")
		for i := 0; i < cols; i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			b.WriteString(" ")
			b.WriteString(padRightVisible(val, widths[i]))
			b.WriteString(" │")
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString("└")
	for i, w := range widths {
		b.WriteString(strings.Repeat("─", w+2))
		if i < cols-1 {
			b.WriteString("┴")
		}
	}
	b.WriteString("┘\n")

	return b.String()
}

func padRight(s string, width int) string {
	l := utf8.RuneCountInString(s)
	if l >= width {
		return s
	}
	return s + strings.Repeat(" ", width-l)
}

// padRightVisible pads based on visible length (ignoring ANSI codes)
func padRightVisible(s string, width int) string {
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}

// visibleLen returns the length of a string ignoring ANSI escape codes
func visibleLen(s string) int {
	length := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

func renderKeyValue(pairs [][]string) string {
	maxKey := 0
	for _, p := range pairs {
		if len(p[0]) > maxKey {
			maxKey = len(p[0])
		}
	}

	var b strings.Builder
	for _, p := range pairs {
		key := padRight(p[0], maxKey)
		b.WriteString(fmt.Sprintf("  %s │ %s\n", colorize(bold, key), p[1]))
	}
	return b.String()
}
