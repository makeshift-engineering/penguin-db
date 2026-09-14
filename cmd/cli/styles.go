package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// Lipgloss Theme Palette (Professional, non-emoji, #92E3A9 anchored)
var (
	ColorPrimary    = lipgloss.Color("#92E3A9")
	ColorPrimaryDim = lipgloss.Color("#5CB87A")
	ColorSurface    = lipgloss.Color("#1A1B26")
	ColorSurfaceAlt = lipgloss.Color("#1E2030")
	ColorText       = lipgloss.Color("#C8D3DC")
	ColorTextDim    = lipgloss.Color("#6B7280")
	ColorError      = lipgloss.Color("#E05252")
	ColorWarning    = lipgloss.Color("#D4A843")
	ColorWhite      = lipgloss.Color("#FFFFFF")
)

// Lipgloss Styles
var (
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StylePromptText = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StylePromptDb = lipgloss.NewStyle().
			Foreground(ColorPrimaryDim)

	StylePromptArrow = lipgloss.NewStyle().
				Foreground(ColorText)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Background(ColorSurface).
			Padding(0, 1)

	StyleRowNumCell = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Padding(0, 1)

	StyleAltRowNumCell = lipgloss.NewStyle().
				Foreground(ColorTextDim).
				Background(ColorSurfaceAlt).
				Padding(0, 1)

	StyleCell = lipgloss.NewStyle().
			Foreground(ColorText).
			Padding(0, 1)

	StyleAltCell = lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorSurfaceAlt).
			Padding(0, 1)

	StyleNullCell = lipgloss.NewStyle().
			Italic(true).
			Foreground(ColorTextDim).
			Padding(0, 1)

	StyleAltNullCell = lipgloss.NewStyle().
				Italic(true).
				Foreground(ColorTextDim).
				Background(ColorSurfaceAlt).
				Padding(0, 1)

	StyleStatusTag = lipgloss.NewStyle().
			Foreground(ColorText)

	StyleTiming = lipgloss.NewStyle().
			Foreground(ColorWarning)

	StyleErrorTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite).
			Background(ColorError).
			Padding(0, 1)

	StyleErrorBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	StyleSuccessBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimaryDim).
			Padding(0, 1).
			MarginTop(1)

	StyleWarningBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorWarning).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	StyleWarningText = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorWarning)

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorTextDim)
)

// FormatPrompt constructs the dynamic colored prompt "penguin (dbname) > "
func FormatPrompt(db string) string {
	if db == "" {
		db = "testdb"
	}
	return fmt.Sprintf("%s %s %s ",
		StylePromptText.Render("penguin"),
		StylePromptDb.Render("("+db+")"),
		StylePromptArrow.Render(">"),
	)
}

// FormatContinuationPrompt constructs the dynamic multi-line continuation prompt "penguin (dbname) -> "
func FormatContinuationPrompt(db string) string {
	if db == "" {
		db = "testdb"
	}
	return fmt.Sprintf("%s %s %s ",
		StylePromptText.Render("penguin"),
		StylePromptDb.Render("("+db+")"),
		StylePromptArrow.Render("->"),
	)
}

// FormatConfirmPrompt constructs the destructive-statement confirmation prompt.
func FormatConfirmPrompt(db string) string {
	if db == "" {
		db = "testdb"
	}
	return fmt.Sprintf("%s %s ",
		StylePromptText.Render("penguin"),
		StyleWarningText.Render("(confirm y/n)"),
	)
}

// bannerArtWidth is the width in columns of the widest ASCII-art title line.
const bannerArtWidth = 78

// RenderBanner renders a startup banner with ANSI shadow logo and system information.
// Below bannerArtWidth (plus border/padding overhead) it falls back to a compact
// text logo instead, since the ASCII art itself can't be word-wrapped without
// destroying it.
func RenderBanner(host string, port int, db string, availWidth int) string {
	compact := availWidth > 0 && availWidth < bannerArtWidth+8

	var top string
	if compact {
		top = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("PENGUINDB")
	} else {
		title := []string{
			`██████╗ ███████╗███╗   ██╗ ██████╗ ██╗   ██╗██╗███╗   ██╗██████╗ ██████╗ `,
			`██╔══██╗██╔════╝████╗  ██║██╔════╝ ██║   ██║██║████╗  ██║██╔══██╗██╔══██╗`,
			`██████╔╝█████╗  ██╔██╗ ██║██║  ███╗██║   ██║██║██╔██╗ ██║██║  ██║██████╔╝`,
			`██╔═══╝ ██╔══╝  ██║╚██╗██║██║   ██║██║   ██║██║██║╚██╗██║██║  ██║██╔══██╗`,
			`██║     ███████╗██║ ╚████║╚██████╔╝╚██████╔╝██║██║ ╚████║██████╔╝██████╔╝`,
			`╚═╝     ╚══════╝╚═╝  ╚═══╝ ╚═════╝  ╚═════╝ ╚═╝╚═╝  ╚═══╝╚═════╝ ╚═════╝ `,
		}
		titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		var sb strings.Builder
		for i, line := range title {
			sb.WriteString(titleStyle.Render(line))
			if i < len(title)-1 {
				sb.WriteString("\n")
			}
		}
		top = sb.String()
	}

	contentWidth := availWidth - 8
	if contentWidth < 20 {
		contentWidth = bannerArtWidth
	}
	wrapStyle := lipgloss.NewStyle().Width(contentWidth)

	statusLine := wrapStyle.Render(fmt.Sprintf(" %s %s  %s %s:%d  %s %s",
		lipgloss.NewStyle().Foreground(ColorPrimary).Render("● Connected"),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("|"),
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Host:"),
		host, port,
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Database:"),
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(db),
	))

	helpShortcuts := wrapStyle.Render(fmt.Sprintf(" %s %s   %s %s   %s %s   %s %s   %s %s",
		StyleHelpKey.Render("\\q"), StyleHelpDesc.Render("quit"),
		StyleHelpKey.Render("\\d"), StyleHelpDesc.Render("tables"),
		StyleHelpKey.Render("\\c <db>"), StyleHelpDesc.Render("use database"),
		StyleHelpKey.Render("\\history"), StyleHelpDesc.Render("past queries"),
		StyleHelpKey.Render("\\h"), StyleHelpDesc.Render("help"),
	))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryDim).
		Padding(1, 2).
		Render(fmt.Sprintf("%s\n\n\n%s\n%s", top, statusLine, helpShortcuts))
}

// Table layout tuning knobs.
const (
	minColWidth = 4
	maxColWidth = 40
)

var reNumeric = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// FormatResultTable builds a Lipgloss-styled data table from QueryResult headers and data rows.
// availWidth is the current terminal width; columns are truncated proportionally to fit it.
// Pass 0 to disable width-fitting (columns render at natural width).
func FormatResultTable(res *QueryResult, availWidth int) string {
	if res == nil {
		return ""
	}

	if res.Error != nil {
		return FormatErrorBox(res.Error.Error(), availWidth)
	}

	if len(res.Columns) == 0 {
		tag := res.CommandTag
		if tag == "" {
			tag = "OK"
		}
		stats := fmt.Sprintf("%s %s   %s %s",
			StyleStatusTag.Render("✓ "+tag),
			lipgloss.NewStyle().Foreground(ColorTextDim).Render("|"),
			StyleTiming.Render(fmt.Sprintf("⏱ %v", res.ExecutionTime)),
			lipgloss.NewStyle().Foreground(ColorPrimary).Render(fmt.Sprintf("[db: %s]", res.ActiveDatabase)),
		)
		return StyleSuccessBox.Render(stats)
	}

	// Column 0 is a synthetic row-number column; the rest mirror res.Columns.
	headers := make([]string, 0, len(res.Columns)+1)
	headers = append(headers, "#")
	for _, col := range res.Columns {
		headers = append(headers, col.Name)
	}
	numCols := len(headers)

	data := make([][]string, 0, len(res.Rows))
	for i, row := range res.Rows {
		newRow := make([]string, 0, numCols)
		newRow = append(newRow, strconv.Itoa(i+1))
		newRow = append(newRow, row...)
		data = append(data, newRow)
	}

	widths, wasTruncated := computeColumnWidths(headers, data, availWidth, numCols)

	rightAlign := make([]bool, numCols)
	rightAlign[0] = true
	for c := 1; c < numCols; c++ {
		rightAlign[c] = isNumericColumn(data, c)
	}

	rendered := make([][]string, len(data))
	for r, row := range data {
		newRow := make([]string, numCols)
		for c := 0; c < numCols; c++ {
			val := ""
			if c < len(row) {
				val = row[c]
			}
			newRow[c] = truncateCell(val, widths[c])
		}
		rendered[r] = newRow
	}

	t := table.New().
		Headers(headers...).
		Rows(rendered...).
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ColorPrimaryDim)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return StyleHeader
			}
			isAltRow := row%2 == 1

			var val string
			if row >= 0 && row < len(rendered) && col >= 0 && col < len(rendered[row]) {
				val = rendered[row][col]
			}

			var style lipgloss.Style
			switch {
			case col == 0:
				if isAltRow {
					style = StyleAltRowNumCell
				} else {
					style = StyleRowNumCell
				}
			case val == "NULL":
				if isAltRow {
					style = StyleAltNullCell
				} else {
					style = StyleNullCell
				}
			case isAltRow:
				style = StyleAltCell
			default:
				style = StyleCell
			}

			if col < len(rightAlign) && rightAlign[col] {
				style = style.Align(lipgloss.Right)
			}
			return style
		})

	var sb strings.Builder
	sb.WriteString(t.Render())

	footer := fmt.Sprintf("%s  %s  %s  %s",
		StyleStatusTag.Render(fmt.Sprintf("✓ %d row(s)", len(res.Rows))),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("|"),
		StyleTiming.Render(fmt.Sprintf("⏱ %v", res.ExecutionTime)),
		lipgloss.NewStyle().Foreground(ColorPrimary).Render(res.CommandTag),
	)
	if wasTruncated {
		footer += "  " + StyleMuted.Render("(columns truncated — widen terminal for full view)")
	}
	sb.WriteString("\n" + footer)

	return sb.String()
}

// computeColumnWidths returns a per-column render width. If availWidth <= 0, columns
// are sized to their natural content width (capped at maxColWidth). Otherwise widths
// are scaled down proportionally so the table fits within availWidth, never shrinking
// below minColWidth. The second return value reports whether any shrinking occurred.
func computeColumnWidths(headers []string, data [][]string, availWidth, numCols int) ([]int, bool) {
	natural := make([]int, numCols)
	for c := 0; c < numCols; c++ {
		w := lipgloss.Width(headers[c])
		for _, row := range data {
			if c < len(row) {
				if cw := lipgloss.Width(row[c]); cw > w {
					w = cw
				}
			}
		}
		if w > maxColWidth {
			w = maxColWidth
		}
		if w < minColWidth {
			w = minColWidth
		}
		natural[c] = w
	}

	if availWidth <= 0 {
		return natural, false
	}

	total := 0
	for _, w := range natural {
		total += w
	}

	// Reserve space for borders (numCols+1 verticals) and cell padding (2 per column).
	overhead := (numCols + 1) + (numCols * 2)
	budget := availWidth - overhead
	if total <= budget || budget <= numCols*minColWidth {
		return natural, false
	}

	scaled := make([]int, numCols)
	for c, w := range natural {
		nw := int(float64(w) / float64(total) * float64(budget))
		if nw < minColWidth {
			nw = minColWidth
		}
		scaled[c] = nw
	}
	return scaled, true
}

// isNumericColumn reports whether every non-null value in a column looks numeric,
// used to right-align that column for readability.
func isNumericColumn(data [][]string, col int) bool {
	seenValue := false
	for _, row := range data {
		if col >= len(row) {
			continue
		}
		v := row[col]
		if v == "" || v == "NULL" {
			continue
		}
		if !reNumeric.MatchString(v) {
			return false
		}
		seenValue = true
	}
	return seenValue
}

// truncateCell shortens a cell's text to fit width, appending an ellipsis when cut.
func truncateCell(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	r := []rune(s)
	if len(r) <= width-1 {
		return s
	}
	return string(r[:width-1]) + "…"
}

// FormatErrorBox renders an error diagnostic panel, wrapped to fit the terminal width.
func FormatErrorBox(errText string, availWidth int) string {
	msg := lipgloss.NewStyle().Foreground(ColorError)
	if availWidth > 10 {
		msg = msg.Width(availWidth - 8)
	}
	content := fmt.Sprintf("%s\n\n%s",
		StyleErrorTitle.Render("ERROR DIAGNOSTIC"),
		msg.Render(errText),
	)
	return StyleErrorBox.Render(content)
}

// FormatConfirmBox renders the "are you sure?" panel shown before a destructive
// statement runs. availWidth wraps the embedded SQL so long statements don't
// overflow the terminal.
func FormatConfirmBox(sql, reason string, availWidth int) string {
	sqlStyle := lipgloss.NewStyle().Foreground(ColorText)
	if availWidth > 10 {
		sqlStyle = sqlStyle.Width(availWidth - 8)
	}
	content := fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
		StyleWarningText.Render("CONFIRMATION REQUIRED"),
		StyleMuted.Render(reason),
		sqlStyle.Render(sql),
		StyleHelpDesc.Render("[y] run it    [n / esc] cancel"),
	)
	return StyleWarningBox.Render(content)
}

// FormatConfirmResolved renders a short one-line record of how a confirmation was resolved.
func FormatConfirmResolved(confirmed bool) string {
	if confirmed {
		return StyleTiming.Render("→ confirmed, executing…")
	}
	return StyleMuted.Render("→ cancelled")
}

// FormatHistory renders the list of previously executed statements, wrapping each
// entry to availWidth so long statements don't push the box past the terminal edge.
func FormatHistory(history []string, availWidth int) string {
	if len(history) == 0 {
		return StyleMuted.Render("No statements executed yet this session.")
	}

	entryStyle := lipgloss.NewStyle().Foreground(ColorText)
	if availWidth > 14 {
		entryStyle = entryStyle.Width(availWidth - 14)
	}

	var sb strings.Builder
	sb.WriteString(StyleTitle.Render("QUERY HISTORY") + "\n\n")
	for i, h := range history {
		idx := StyleHelpKey.Render(fmt.Sprintf("[%2d]", i+1))
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, idx+" ", entryStyle.Render(h)) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryDim).
		Padding(1, 2).
		Render(strings.TrimRight(sb.String(), "\n"))
}

// FormatHelpPanel renders the SQL syntax and commands helper reference card.
func FormatHelpPanel(availWidth int) string {
	title := StyleTitle.Render("PENGUINDB SQL COMMAND CHEAT SHEET")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"SELECT col1, col2 FROM tbl WHERE cond ORDER BY col ASC;", "Query records with filters and sorting"},
		{"INSERT INTO tbl VALUES (val1, val2), (...);", "Insert single or multiple row tuples"},
		{"UPDATE tbl SET col = val WHERE cond;", "Modify column values of matching rows"},
		{"DELETE FROM tbl WHERE cond;", "Delete matching row tuples from table"},
		{"CREATE DATABASE db_name; / USE db_name;", "Create or switch active target database"},
		{"CREATE TABLE tbl (id INT PRIMARY KEY, name VARCHAR(50));", "Create new database table schema"},
		{"DROP TABLE tbl; / DROP DATABASE db_name;", "Drop table or database entity"},
		{"\\d or \\dt", "Shortcut: list all tables in active catalog"},
		{"\\c db_name", "Shortcut: switch active working database"},
		{"\\history", "Shortcut: list previously executed statements"},
		{"\\export file.csv", "Shortcut: export the last result set to CSV"},
		{"\\clear", "Clear TUI screen terminal output"},
		{"\\q or exit", "Disconnect wire client and exit PenguinDB CLI"},
	}

	const cmdColWidth = 40
	descStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	if descWidth := availWidth - cmdColWidth - 8; descWidth > 10 {
		descStyle = descStyle.Width(descWidth)
	}

	var sb strings.Builder
	sb.WriteString(title + "\n\n")

	for _, c := range commands {
		key := StyleHelpKey.Width(cmdColWidth).Render(c.cmd)
		desc := descStyle.Render(c.desc)
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, key, desc) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryDim).
		Padding(1, 2).
		Render(sb.String())
}
