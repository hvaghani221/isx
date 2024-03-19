package shell

import (
	"bytes"
	"regexp"
	"strings"
)

// Line represents a single line ending with \n
type Line []byte

type Output struct {
	Stdout []Line
	Stderr []Line
}

type Command interface {
	Execute(in Input) <-chan Output
	Close()
}

type Input interface {
	Listen() (result <-chan []Line, cancelFunc func())
	Close()
}

func NewLines(data []byte) []Line {
	split := bytes.SplitAfter(data, []byte{'\n'})
	lines := make([]Line, len(split))

	for i, bl := range split {
		lines[i] = Line(bl)
	}

	return lines
}

func Exec(command string, mode Mode) Command {
	if strings.TrimSpace(command) == "" {
		return NewCat()
	}

	switch mode {
	case Iterative:
		return NewIterativeCmd(parseCommand(command))
	case Live:
		return NewLiveCmd(parseCommand(command))
	case Snapshot:
		return NewSnapshotCmd(parseCommand(command))
	default:
		panic("unknown mode")
	}

}

// func countLines(data []byte) int {
// 	return bytes.Count(data, []byte{'\n'})
// }

var pattern = regexp.MustCompile(`"[^"]*"|'[^']*'|\S+`)

func parseCommand(cmd string) []string {
	return pattern.FindAllString(cmd, -1)
}
