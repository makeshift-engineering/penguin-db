package main

import (
	"fmt"
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

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StyleHelpDesc = lipgloss.NewStyle().
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

// RenderBanner renders a startup banner with ANSI shadow logo and system information.
func RenderBanner(host string, port int, db string) string {
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

	statusLine := fmt.Sprintf(" %s %s  %s %s:%d  %s %s",
		lipgloss.NewStyle().Foreground(ColorPrimary).Render("● Connected"),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("|"),
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Host:"),
		host, port,
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("Database:"),
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(db),
	)

	helpShortcuts := fmt.Sprintf(" %s %s   %s %s   %s %s   %s %s",
		StyleHelpKey.Render("\\q"), StyleHelpDesc.Render("quit"),
		StyleHelpKey.Render("\\d"), StyleHelpDesc.Render("tables"),
		StyleHelpKey.Render("\\c <db>"), StyleHelpDesc.Render("use database"),
		StyleHelpKey.Render("\\h"), StyleHelpDesc.Render("help"),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryDim).
		Padding(1, 2).
		Render(fmt.Sprintf("%s\n\n\n%s\n%s", sb.String(), statusLine, helpShortcuts))
}

// FormatResultTable builds a Lipgloss-styled data table from QueryResult headers and data rows.
func FormatResultTable(res *QueryResult) string {
	if res == nil {
		return ""
	}

	if res.Error != nil {
		return FormatErrorBox(res.Error.Error())
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

	var headers []string
	for _, col := range res.Columns {
		headers = append(headers, col.Name)
	}

	var data [][]string
	for _, row := range res.Rows {
		var newRow []string
		for _, cell := range row {
			if cell == "NULL" {
				// We'll style NULL in the table styling function if possible, but passing string for now
				newRow = append(newRow, "NULL")
			} else {
				newRow = append(newRow, cell)
			}
		}
		data = append(data, newRow)
	}

	t := table.New().
		Headers(headers...).
		Rows(data...).
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ColorPrimaryDim)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return StyleHeader
			}
			isAltRow := (row%2 == 1)

			// Try to check if it's NULL (not perfectly accurate if data literally is "NULL", but sufficient here)
			var val string
			if row >= 0 && row < len(data) && col >= 0 && col < len(data[row]) {
				val = data[row][col]
			}

			if val == "NULL" {
				if isAltRow {
					return StyleAltNullCell
				}
				return StyleNullCell
			}

			if isAltRow {
				return StyleAltCell
			}
			return StyleCell
		})

	var sb strings.Builder
	sb.WriteString(t.Render())

	footer := fmt.Sprintf("%s  %s  %s  %s",
		StyleStatusTag.Render(fmt.Sprintf("✓ %d row(s)", len(res.Rows))),
		lipgloss.NewStyle().Foreground(ColorTextDim).Render("|"),
		StyleTiming.Render(fmt.Sprintf("⏱ %v", res.ExecutionTime)),
		lipgloss.NewStyle().Foreground(ColorPrimary).Render(fmt.Sprintf("%s", res.CommandTag)),
	)
	sb.WriteString("\n" + footer)

	return sb.String()
}

// FormatErrorBox renders an error diagnostic panel.
func FormatErrorBox(errText string) string {
	content := fmt.Sprintf("%s\n\n%s",
		StyleErrorTitle.Render("ERROR DIAGNOSTIC"),
		lipgloss.NewStyle().Foreground(ColorError).Render(errText),
	)
	return StyleErrorBox.Render(content)
}

// FormatHelpPanel renders the SQL syntax and commands helper reference card.
func FormatHelpPanel() string {
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
		{"\\clear", "Clear TUI screen terminal output"},
		{"\\q or exit", "Disconnect wire client and exit PenguinDB CLI"},
	}

	var sb strings.Builder
	sb.WriteString(title + "\n\n")

	for _, c := range commands {
		key := StyleHelpKey.Width(35).Render(c.cmd)
		desc := StyleHelpDesc.Render(c.desc)
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, key, desc) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimaryDim).
		Padding(1, 2).
		Render(sb.String())
}
