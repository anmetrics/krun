package main

import (
	"fmt"
	"os"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

var noColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

func colorize(color, s string) string {
	if noColor {
		return s
	}
	return color + s + reset
}

func statusColor(status string) string {
	switch status {
	case "active", "online":
		return colorize(green+bold, "online")
	case "inactive", "stopped":
		return colorize(red, "stopped")
	case "failed":
		return colorize(red+bold, "errored")
	case "activating":
		return colorize(yellow, "launching")
	case "deactivating":
		return colorize(yellow, "stopping")
	default:
		return colorize(gray, status)
	}
}

func printSuccess(format string, args ...any) {
	fmt.Printf(colorize(green, "✓")+" "+format+"\n", args...)
}

func printError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, colorize(red, "✗")+" "+format+"\n", args...)
}

func printInfo(format string, args ...any) {
	fmt.Printf(colorize(cyan, "→")+" "+format+"\n", args...)
}
