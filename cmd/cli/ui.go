package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"charm.land/lipgloss/v2"
)

// QueryResultMsg is emitted when a SQL query execution completes.
type QueryResultMsg struct {
	Result *QueryResult
}

// Model defines the Bubbletea application state machine.
type Model struct {
	client      *PGClient
	textInput   textinput.Model
	viewport    viewport.Model
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
}

// NewModel instantiates a new Bubbletea TUI Model.
func NewModel(client *PGClient, host string, port int, activeDb string) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter SQL statement (ending with ;) or \\h for help..."
	ti.Focus()
	ti.CharLimit = 2000
	ti.Width = 80
	ti.Prompt = FormatPrompt(activeDb)

	vp := viewport.New(80, 20)
	vp.YPosition = 0

	m := Model{
		client:     client,
		textInput:  ti,
		viewport:   vp,
		history:    make([]string, 0),
		historyIdx: -1,
		host:       host,
		port:       port,
		activeDb:   activeDb,
	}

	m.appendOutput(RenderBanner(host, port, activeDb))
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
			m.textInput.Width = m.width - 25
		}
		m.viewport.Width = msg.Width
		if m.height > 6 {
			m.viewport.Height = msg.Height - 6
		}
		m.viewport.SetContent(m.output)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if len(m.multiBuffer) > 0 {
				m.multiBuffer = nil
				m.textInput.SetValue("")
				m.textInput.Prompt = FormatPrompt(m.activeDb)
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
				switch strings.ToLower(trimmed) {
				case "\\q", "exit", "quit":
					m.client.Close()
					return m, tea.Quit

				case "\\h", "help":
					m.appendOutput("\n" + FormatHelpPanel())
					m.textInput.SetValue("")
					m.historyIdx = -1
					return m, nil

				case "\\clear", "clear":
					m.output = ""
					m.appendOutput(RenderBanner(m.host, m.port, m.activeDb))
					m.textInput.SetValue("")
					m.historyIdx = -1
					return m, nil

				case "\\d", "\\dt":
					val = "SELECT table_name FROM information_schema.tables;"
					trimmed = val

				default:
					if strings.HasPrefix(strings.ToLower(trimmed), "\\c ") {
						parts := strings.Fields(trimmed)
						if len(parts) >= 2 {
							val = fmt.Sprintf("USE %s;", parts[1])
							trimmed = val
						}
					}
				}
			} else {
				// Allow clearing or quitting during multi-line input
				if strings.ToLower(trimmed) == "\\clear" {
					m.multiBuffer = nil
					m.output = ""
					m.appendOutput(RenderBanner(m.host, m.port, m.activeDb))
					m.textInput.SetValue("")
					m.textInput.Prompt = FormatPrompt(m.activeDb)
					m.historyIdx = -1
					return m, nil
				}
				if strings.ToLower(trimmed) == "\\q" || strings.ToLower(trimmed) == "exit" || strings.ToLower(trimmed) == "quit" {
					m.client.Close()
					return m, tea.Quit
				}
			}

			m.multiBuffer = append(m.multiBuffer, val)
			m.textInput.SetValue("")
			m.historyIdx = -1

			// Display input line in viewport with appropriate prompt
			if len(m.multiBuffer) == 1 {
				m.appendOutput(fmt.Sprintf("\n%s %s", FormatPrompt(m.activeDb), val))
			} else {
				m.appendOutput(fmt.Sprintf("%s %s", FormatContinuationPrompt(m.activeDb), val))
			}

			fullSql := strings.TrimSpace(strings.Join(m.multiBuffer, "\n"))

			// Check if complete statement ends with ';'
			if strings.HasSuffix(fullSql, ";") {
				m.history = append(m.history, fullSql)
				m.multiBuffer = nil
				m.textInput.Prompt = FormatPrompt(m.activeDb)
				m.loading = true
				return m, execQueryCmd(m.client, fullSql)
			}

			// Statement incomplete: switch to continuation prompt
			m.textInput.Prompt = FormatContinuationPrompt(m.activeDb)
			return m, nil
		}

	case QueryResultMsg:
		m.loading = false
		if msg.Result != nil {
			if msg.Result.ActiveDatabase != "" {
				m.activeDb = msg.Result.ActiveDatabase
				m.textInput.Prompt = FormatPrompt(m.activeDb)
			}
			m.appendOutput(FormatResultTable(msg.Result))
		}
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m *Model) appendOutput(text string) {
	m.output += text + "\n"
	m.viewport.SetContent(m.output)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	var statusIndicator string
	if m.loading {
		statusIndicator = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render(" Executing query...")
	} else {
		statusIndicator = lipgloss.NewStyle().Foreground(ColorTextDim).Render(fmt.Sprintf(" db: %s", m.activeDb))
	}

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

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		divider,
		inputBar,
	)
}

func execQueryCmd(client *PGClient, sql string) tea.Cmd {
	return func() tea.Msg {
		res, _ := client.ExecuteQuery(sql)
		return QueryResultMsg{Result: res}
	}
}
