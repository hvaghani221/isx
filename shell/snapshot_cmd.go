package shell

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os/exec"
	"time"
)

type snapshot struct {
	cmd        []string
	cancelChan chan struct{}
}

func NewSnapshotCmd(cmd []string) *snapshot {
	return &snapshot{
		cmd:        cmd,
		cancelChan: make(chan struct{}),
	}
}

func (l *snapshot) Execute(in Input) <-chan Output {
	ch := make(chan Output)

	go func() {
		inCh, cancelListen := in.Listen()
		var bufOut bytes.Buffer
		var bufErr bytes.Buffer

		ctx, cancelCtx := context.WithCancel(context.Background())
		lineBuffer := make([]Line, 0, 1024)

		ticker := time.NewTicker(tickInterval)
		bufoutLen, buferrLen := 0, 0
		defer ticker.Stop()
		for {
			select {
			case <-l.cancelChan:
				cancelListen()
				cancelCtx()
				close(ch)
				return
			case lines := <-inCh:
				lineBuffer = append(lineBuffer, lines...)
				bufOut.Reset()
				bufErr.Reset()
				cmd := exec.CommandContext(ctx, l.cmd[0], l.cmd[1:]...)
				cmd.Stdin = NewMultilineReader(lineBuffer)
				cmd.Stdout = &bufOut
				cmd.Stderr = &bufErr

				if err := cmd.Run(); err != nil {
					if _, ok := err.(*exec.ExitError); !(ok || errors.Is(err, context.Canceled)) {
						ch <- Output{
							Stdout: nil,
							Stderr: NewLines(bufErr.Bytes()),
						}
						log.Printf("error executing command of type %T: %v\n", err, err)
						continue
					}
				}
			case <-ticker.C:
				if bufoutLen == bufOut.Len() && buferrLen == bufErr.Len() {
					continue
				}
				bufoutLen = bufOut.Len()
				buferrLen = bufErr.Len()
				ch <- Output{
					Stdout: NewLines(bufOut.Bytes()),
					Stderr: NewLines(bufErr.Bytes()),
				}
			}
		}
	}()
	return ch
}

func (l *snapshot) Close() {
	close(l.cancelChan)
}

var _ Command = (*snapshot)(nil)
