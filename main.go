package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	self        *tea.Program
	sh          *shell
	input       strings.Builder
	viewport    viewport.Model
	cmdInput    textinput.Model
	ctx         context.Context
	cancelShell func()
}

var viewportKeyMap = viewport.KeyMap{
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "½ page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "½ page down"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "ctrl+k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "ctrl+j"),
		key.WithHelp("↓/j", "down"),
	),
}

func newModel(sh *shell) *model {
	ctx, cancel := context.WithCancel(context.Background())
	sh.Start(ctx)
	return &model{
		sh:          sh,
		cmdInput:    textinput.New(),
		ctx:         ctx,
		cancelShell: cancel,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.cmdInput.Focus(), m.executeCommandFull)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelShell()
			return m, tea.Quit
		case "enter":
			m.input.Reset()
			m.viewport.SetContent("")
			// cmds = append(cmds, tea.ClearScreen)
			cmds = append(cmds, m.executeCommandFull)
		}
	case tea.WindowSizeMsg:
		cmdHeight := lipgloss.Height(m.renderCommandLine())
		m.viewport = viewport.New(msg.Width, msg.Height-cmdHeight*2)
		m.viewport.KeyMap = viewportKeyMap
		// m.viewport.HighPerformanceRendering = highPerformance
		m.viewport.YPosition = 1
		m.cmdInput.Width = msg.Width - 3
		// cmds = append(cmds, viewport.Sync(m.viewport))
	case outputWithClear:
		m.input.Reset()
		m.input.Write(msg)
		// atBottom := m.viewport.AtBottom()
		m.viewport.SetContent(m.input.String())
		// if atBottom {
		// 	m.viewport.GotoBottom()
		// }
	case []byte:
		// m.input.Reset()
		m.input.Write(msg)
		atBottom := m.viewport.AtBottom()
		m.viewport.SetContent(m.input.String())
		if atBottom {
			m.viewport.GotoBottom()
		}

	}

	m.cmdInput, cmd = m.cmdInput.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) executeCommand() tea.Msg {
	cmd := m.cmdInput.Value()
	if strings.TrimSpace(cmd) == "" {
		return m.echoCommand()
	}

	op := m.sh.ExecuteCommandLineByLine(m.ctx, cmd)
	go func() {
		for line := range op {
			m.self.Send(line)
		}
	}()

	return nil
}

func (m *model) echoCommand() tea.Msg {
	op := m.sh.ListenInput(m.ctx)
	go func() {
		for line := range op {
			m.self.Send(line)
		}
	}()

	return nil
}

type outputWithClear []byte

func (m *model) executeCommandFull() tea.Msg {
	cmd := m.cmdInput.Value()
	if strings.TrimSpace(cmd) == "" {
		return m.echoCommand()
	}

	op := m.sh.ExecuteCommand(m.ctx, cmd)
	go func() {
		for line := range op {
			m.self.Send(outputWithClear(line))
		}
	}()

	return nil
}

func (m *model) View() string {
	// return m.renderLine()
	return fmt.Sprintf("%s\n%s\n%s", m.renderViewport(), m.renderLine(), m.renderCommandLine())
}

func (m *model) renderLine() string {
	return strings.Repeat("─", m.viewport.Width)
}

func (m *model) renderViewport() string {
	return m.viewport.View()
}

func (m *model) renderCommandLine() string {
	return m.cmdInput.View()
}

func main() {
	fstat, err := os.Stdin.Stat()
	if err != nil {
		panic(err)
	}

	if fstat.Mode()&os.ModeCharDevice != 0 {
		fmt.Println("stdin should be piped")
		os.Exit(1)
	}

	sh := NewShell(os.Stdin)
	m := newModel(sh)

	file, err := tea.LogToFile("debug.log", "")
	if err != nil {
		panic(err)
	}
	_ = file.Truncate(0)
	defer file.Close()

	program := tea.NewProgram(m, tea.WithAltScreen())
	m.self = program
	if _, err := program.Run(); err != nil {
		panic(err)
	}
}
