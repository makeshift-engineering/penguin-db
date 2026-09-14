package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// QueryResultMsg is emitted when a SQL query execution completes.
type QueryResultMsg struct {
	Result *QueryResult
}

// Heuristics for flagging statements worth a confirmation prompt before they run.
var (
	reDropOrTruncate = regexp.MustCompile(`(?i)^\s*(DROP\s+(TABLE|DATABASE)|TRUNCATE)\b`)
	reDeleteOrUpdate = regexp.MustCompile(`(?i)^\s*(DELETE\s+FROM|UPDATE)\b`)
	reHasWhere       = regexp.MustCompile(`(?i)\bWHERE\b`)
)

// isDestructive flags statements that can cause irreversible or broad data loss,
// returning a human-readable reason to show in the confirmation panel.
func isDestructive(sql string) (bool, string) {
	switch {
	case reDropOrTruncate.MatchString(sql):
		return true, "This permanently drops or empties a table or database."
	case reDeleteOrUpdate.MatchString(sql) && !reHasWhere.MatchString(sql):
		return true, "No WHERE clause detected — this will affect every row."
	default:
		return false, ""
	}
}

// Model defines the Bubbletea application state machine.
type Model struct {
	client      *PGClient
	textInput   textinput.Model
	viewport    viewport.Model
	spinner     spinner.Model
	history     []string
	historyIdx  int
	output      string
	multiBuffer []string
	loading     bool
	host        string
	port        int
	activeDb    string
	width       int
	height      int
	lastResult  *QueryResult

	// confirmSQL holds a pending destructive statement awaiting y/n confirmation.
	confirmSQL  string
	confirmDesc string
}

// NewModel instantiates a new Bubbletea TUI Model.
func NewModel(client *PGClient, host string, port int, activeDb string) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter SQL statement (ending with ;) or \\h for help..."
	ti.Focus()
	ti.CharLimit = 2000

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SoftWrap = true // wrap long scrollback lines instead of letting them run off-screen

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorPrimary)

	m := Model{
		client:     client,
		textInput:  ti,
		viewport:   vp,
		spinner:    sp,
		history:    make([]string, 0),
		historyIdx: -1,
		host:       host,
		port:       port,
		activeDb:   activeDb,
	}
	m.setPrompt(FormatPrompt(activeDb))

	m.appendOutput(RenderBanner(host, port, activeDb, 0))
	return m
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 25 {
			m.syncInputWidth()
		}
		m.viewport.SetWidth(msg.Width)
		if m.height > 4 {
			m.viewport.SetHeight(msg.Height - 4)
		}
		m.viewport.SetContent(m.output)

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		// A destructive statement is awaiting explicit confirmation; every
		// other key binding is suspended until it's resolved.
		if m.confirmSQL != "" {
			switch strings.ToLower(msg.String()) {
			case "y":
				sql := m.confirmSQL
				m.confirmSQL = ""
				m.confirmDesc = ""
				m.setPrompt(FormatPrompt(m.activeDb))
				m.history = append(m.history, sql)
				m.appendOutput(FormatConfirmResolved(true))
				m.loading = true
				return m, tea.Batch(execQueryCmd(m.client, sql), m.spinner.Tick)
			default:
				m.confirmSQL = ""
				m.confirmDesc = ""
				m.setPrompt(FormatPrompt(m.activeDb))
				m.appendOutput(FormatConfirmResolved(false))
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			if len(m.multiBuffer) > 0 {
				m.multiBuffer = nil
				m.textInput.SetValue("")
				m.setPrompt(FormatPrompt(m.activeDb))
				m.appendOutput("^C")
				return m, nil
			}
			m.client.Close()
			return m, tea.Quit

		case "up":
			if len(m.history) > 0 {
				if m.historyIdx == -1 {
					m.historyIdx = len(m.history) - 1
				} else if m.historyIdx > 0 {
					m.historyIdx--
				}
				m.textInput.SetValue(m.history[m.historyIdx])
				m.textInput.SetCursor(len(m.textInput.Value()))
			}

		case "down":
			if m.historyIdx != -1 {
				if m.historyIdx < len(m.history)-1 {
					m.historyIdx++
					m.textInput.SetValue(m.history[m.historyIdx])
					m.textInput.SetCursor(len(m.textInput.Value()))
				} else {
					m.historyIdx = -1
					m.textInput.SetValue("")
				}
			}

		case "enter":
			val := m.textInput.Value()
			trimmed := strings.TrimSpace(val)

			if len(m.multiBuffer) == 0 && trimmed == "" {
				return m, nil
			}

			// Handle meta-commands when no multi-line buffer is pending
			if len(m.multiBuffer) == 0 {
				lower := strings.ToLower(trimmed)
				switch lower {
				case "\\q", "exit", "quit":
					m.client.Close()
					return m, tea.Quit

				case "\\h", "help":
					m.appendOutput("\n" + FormatHelpPanel(m.termWidth()))
					m.textInput.SetValue("")
					m.historyIdx = -1
					return m, nil

				case "\\clear", "clear":
					m.output = ""
					m.appendOutput(RenderBanner(m.host, m.port, m.activeDb, m.termWidth()))
					m.textInput.SetValue("")
					m.historyIdx = -1
					return m, nil

				case "\\d", "\\dt":
					val = "SELECT table_name FROM information_schema.tables;"
					trimmed = val

				case "\\history":
					m.appendOutput("\n" + FormatHistory(m.history, m.termWidth()))
					m.textInput.SetValue("")
					m.historyIdx = -1
					return m, nil

				default:
					switch {
					case strings.HasPrefix(lower, "\\c "):
						parts := strings.Fields(trimmed)
						if len(parts) >= 2 {
							val = fmt.Sprintf("USE %s;", parts[1])
							trimmed = val
						}
					case strings.HasPrefix(lower, "\\export "):
						filename := strings.TrimSpace(trimmed[len("\\export "):])
						m.appendOutput(m.exportLastResultCSV(filename))
						m.textInput.SetValue("")
						m.historyIdx = -1
						return m, nil
					}
				}
			} else {
				// Allow clearing or quitting during multi-line input
				switch strings.ToLower(trimmed) {
				case "\\clear":
					m.multiBuffer = nil
					m.output = ""
					m.appendOutput(RenderBanner(m.host, m.port, m.activeDb, m.termWidth()))
					m.textInput.SetValue("")
					m.setPrompt(FormatPrompt(m.activeDb))
					m.historyIdx = -1
					return m, nil
				case "\\q", "exit", "quit":
					m.client.Close()
					return m, tea.Quit
				}
			}

			m.multiBuffer = append(m.multiBuffer, val)
			m.textInput.SetValue("")
			m.historyIdx = -1

			// Display input line in viewport with appropriate prompt. The viewport's
			// SoftWrap handles long statements — no manual wrapping needed here.
			if len(m.multiBuffer) == 1 {
				m.appendOutput(fmt.Sprintf("\n%s %s", FormatPrompt(m.activeDb), val))
			} else {
				m.appendOutput(fmt.Sprintf("%s %s", FormatContinuationPrompt(m.activeDb), val))
			}

			fullSql := strings.TrimSpace(strings.Join(m.multiBuffer, "\n"))

			// Check if complete statement ends with ';'
			if strings.HasSuffix(fullSql, ";") {
				m.multiBuffer = nil
				m.setPrompt(FormatPrompt(m.activeDb))

				if destructive, reason := isDestructive(fullSql); destructive {
					m.confirmSQL = fullSql
					m.confirmDesc = reason
					m.setPrompt(FormatConfirmPrompt(m.activeDb))
					m.appendOutput(FormatConfirmBox(fullSql, reason, m.termWidth()))
					return m, nil
				}

				m.history = append(m.history, fullSql)
				m.loading = true
				return m, tea.Batch(execQueryCmd(m.client, fullSql), m.spinner.Tick)
			}

			// Statement incomplete: switch to continuation prompt
			m.setPrompt(FormatContinuationPrompt(m.activeDb))
			return m, nil
		}

	case QueryResultMsg:
		m.loading = false
		m.lastResult = msg.Result
		if msg.Result != nil {
			if msg.Result.ActiveDatabase != "" {
				m.activeDb = msg.Result.ActiveDatabase
				m.setPrompt(FormatPrompt(m.activeDb))
			}
			m.appendOutput(FormatResultTable(msg.Result, m.termWidth()))
		}
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd)
}

// statusWidth is the fixed column budget reserved for the status indicator
// beside the input box (loading spinner, confirm hint, or "db: x │ NN%").
// Longer status content is truncated rather than allowed to overflow.
const statusWidth = 30

// termWidth returns the current terminal width, falling back to a sane default
// before the first tea.WindowSizeMsg arrives.
func (m Model) termWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// setPrompt updates the text input's prompt and immediately resyncs the input's
// width, since the prompt's rendered width (e.g. "penguin (dbname) > ") changes
// whenever the active database or input mode changes — not just on resize.
func (m *Model) setPrompt(prompt string) {
	m.textInput.Prompt = prompt
	m.syncInputWidth()
}

// syncInputWidth sizes the text input's editable area so that prompt + input +
// the reserved status-indicator budget never exceed the terminal width.
func (m *Model) syncInputWidth() {
	avail := m.termWidth() - lipgloss.Width(m.textInput.Prompt) - statusWidth - 1
	if avail < 10 {
		avail = 10
	}
	m.textInput.SetWidth(avail)
}

func (m *Model) appendOutput(text string) {
	m.output += text + "\n"
	m.viewport.SetContent(m.output)
	m.viewport.GotoBottom()
}

// exportLastResultCSV writes the most recent result set to a CSV file on disk.
func (m Model) exportLastResultCSV(filename string) string {
	if filename == "" {
		return FormatErrorBox("usage: \\export <filename.csv>", m.termWidth())
	}
	res := m.lastResult
	if res == nil || res.Error != nil || len(res.Columns) == 0 {
		return FormatErrorBox("no exportable result set — run a SELECT first", m.termWidth())
	}

	f, err := os.Create(filename)
	if err != nil {
		return FormatErrorBox(fmt.Sprintf("could not create file: %v", err), m.termWidth())
	}
	defer f.Close()

	w := csv.NewWriter(f)
	headers := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		headers[i] = c.Name
	}
	if err := w.Write(headers); err != nil {
		return FormatErrorBox(fmt.Sprintf("failed writing csv: %v", err), m.termWidth())
	}
	for _, row := range res.Rows {
		if err := w.Write(row); err != nil {
			return FormatErrorBox(fmt.Sprintf("failed writing csv: %v", err), m.termWidth())
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return FormatErrorBox(fmt.Sprintf("failed writing csv: %v", err), m.termWidth())
	}

	return StyleSuccessBox.Render(fmt.Sprintf("Exported %d row(s) to %s", len(res.Rows), filename))
}

func (m Model) View() tea.View {
	var statusIndicator string
	switch {
	case m.loading:
		statusIndicator = StyleWarningText.Render(fmt.Sprintf(" %s Executing query...", m.spinner.View()))
	case m.confirmSQL != "":
		statusIndicator = StyleWarningText.Render(" awaiting confirmation (y/n)")
	default:
		statusIndicator = StyleMuted.Render(fmt.Sprintf(" db: %s │ %3.0f%%", m.activeDb, m.viewport.ScrollPercent()*100))
	}
	statusIndicator = lipgloss.NewStyle().MaxWidth(statusWidth).Render(statusIndicator)

	inputBar := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.textInput.View(),
		statusIndicator,
	)

	w := m.width
	if w <= 0 {
		w = 80
	}

	divider := lipgloss.NewStyle().
		Foreground(ColorPrimaryDim).
		Render(strings.Repeat("─", w))

	return tea.NewView(lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		divider,
		inputBar,
	))
}

func execQueryCmd(client *PGClient, sql string) tea.Cmd {
	return func() tea.Msg {
		res, _ := client.ExecuteQuery(sql)
		return QueryResultMsg{Result: res}
	}
}
