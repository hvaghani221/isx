package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hvaghani221/isx/shell"
)

type shellConfig struct {
	in     shell.Input
	cmd    shell.Command
	mode   shell.Mode
	id     atomic.Int32
	output string
}

type model struct {
	self *tea.Program
	// input    strings.Builder
	viewport viewport.Model
	cmdInput textinput.Model
	shell    shellConfig
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

func NewProgram(in shell.Input) (*model, *tea.Program) {
	m := NewModel(in)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stdout))
	m.self = p

	return m, p
}

func NewModel(in shell.Input) *model {
	return &model{
		cmdInput: textinput.New(),
		shell: shellConfig{
			in:   in,
			mode: shell.Snapshot,
		},
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.cmdInput.Focus(), textinput.Blink, m.executeCommand)
}

var count = 0

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":

			return m, tea.Quit
		case "enter":
			// m.input.Reset()
			m.viewport.SetContent("")
			// cmds = append(cmds, tea.ClearScreen)
			cmds = append(cmds, m.executeCommand)
		}
	case tea.WindowSizeMsg:
		cmdHeight := lipgloss.Height(m.renderCommandLine())
		m.viewport = viewport.New(msg.Width, msg.Height-cmdHeight*2)
		m.viewport.HighPerformanceRendering = true
		m.viewport.KeyMap = viewportKeyMap
		m.viewport.YPosition = 0
		m.cmdInput.Width = msg.Width - 3
		cmds = append(cmds, viewport.Sync(m.viewport))
	case output:
		// m.input.Reset()
		// m.input.Write(msg)
		if msg.id != m.shell.id.Load() {
			return m, nil
		}
		// atBottom := m.viewport.AtBottom()

		if len(msg.op.Stdout) == 0 {
			m.viewport.SetContent(join(msg.op.Stderr))
		} else {
			op := join(msg.op.Stdout)
			m.shell.output = op
			m.viewport.SetContent(op)
			// m.viewport.SetContent(fmt.Sprintf("%d bytes", len(op)))
			cmds = append(cmds, viewport.Sync(m.viewport))
		}
		// if atBottom {
		// 	m.viewport.GotoBottom()
		// }
	}

	m.cmdInput, cmd = m.cmdInput.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func join(strs []shell.Line) string {
	var sb strings.Builder
	for _, str := range strs {
		sb.Write(str)
		// sb.WriteByte('\n')
	}
	return sb.String()
}

type output struct {
	id int32
	op shell.Output
}

func (m *model) executeCommand() tea.Msg {
	if m.shell.cmd != nil {
		m.shell.cmd.Close()
	}

	id := m.shell.id.Add(1)
	cmd := shell.Exec(m.cmdInput.Value(), m.shell.mode)
	op := cmd.Execute(m.shell.in)
	go func() {
		for line := range op {
			count++
			m.self.Send(output{id: id, op: line})
			m.self.Send(line)
		}
	}()
	return nil
}

func (m *model) View() string {
	// return m.renderLine()
	return fmt.Sprintf("%s\n%s\n%s", m.renderViewport(), m.renderLine(), m.renderCommandLine())
}

func (m *model) renderLine() string {
	return fmt.Sprint(strings.Repeat("─", m.viewport.Width/2), count)
}

func (m *model) renderViewport() string {
	return m.viewport.View()
}

func (m *model) renderCommandLine() string {
	return m.cmdInput.View()
}

func (m *model) WriteOutputTo(w io.Writer) {
	_, _ = io.Copy(w, strings.NewReader(m.shell.output))
}
