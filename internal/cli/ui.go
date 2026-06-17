package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Minimal terminal UI helpers. Colors are disabled when NO_COLOR is set or the
// output is not a terminal-like stream.

var colorEnabled = os.Getenv("NO_COLOR") == ""

func colorize(code, text string) string {
	if !colorEnabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func bold(text string) string   { return colorize("1", text) }
func green(text string) string  { return colorize("32", text) }
func red(text string) string    { return colorize("31", text) }
func yellow(text string) string { return colorize("33", text) }
func dim(text string) string    { return colorize("2", text) }

func printErr(format string, args ...any) {
	fmt.Fprintln(os.Stderr, red("error: ")+fmt.Sprintf(format, args...))
}

func printInfo(format string, args ...any) {
	fmt.Println(fmt.Sprintf(format, args...))
}

func printSuccess(format string, args ...any) {
	fmt.Println(green("✓ ") + fmt.Sprintf(format, args...))
}

func printNote(format string, args ...any) {
	fmt.Println(yellow("! ") + fmt.Sprintf(format, args...))
}

var stdin = bufio.NewReader(os.Stdin)

// prompt reads a single line, returning def when the input is empty.
func prompt(label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// promptChoice asks the user to pick one of the options by number.
func promptChoice(label string, options []string, def int) string {
	fmt.Println(label)
	for i, opt := range options {
		marker := "  "
		if i == def {
			marker = "> "
		}
		fmt.Printf("  %s%d) %s\n", marker, i+1, opt)
	}
	for {
		raw := prompt("Select", fmt.Sprintf("%d", def+1))
		var idx int
		if _, err := fmt.Sscanf(raw, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
			return options[idx-1]
		}
		printErr("enter a number between 1 and %d", len(options))
	}
}

func confirm(label string, def bool) bool {
	suffix := "y/N"
	if def {
		suffix = "Y/n"
	}
	raw := strings.ToLower(prompt(fmt.Sprintf("%s (%s)", label, suffix), ""))
	if raw == "" {
		return def
	}
	return raw == "y" || raw == "yes"
}

// table renders aligned columns.
func table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	printRow := func(cells []string, header bool) {
		parts := make([]string, len(cells))
		for i, cell := range cells {
			pad := widths[i] - len(cell)
			if pad < 0 {
				pad = 0
			}
			text := cell + strings.Repeat(" ", pad)
			if header {
				text = bold(text)
			}
			parts[i] = text
		}
		fmt.Println(strings.Join(parts, "  "))
	}
	printRow(headers, true)
	for _, row := range rows {
		printRow(row, false)
	}
}
