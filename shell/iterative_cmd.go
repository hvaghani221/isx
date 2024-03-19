package shell

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os/exec"
)

type iterative struct {
	cmd        []string
	cancelChan chan struct{}
}

func NewIterativeCmd(cmd []string) *iterative {
	return &iterative{
		cmd:        cmd,
		cancelChan: make(chan struct{}),
	}
}

func (l *iterative) Execute(in Input) <-chan Output {
	ch := make(chan Output)

	go func() {
		inCh, cancelListen := in.Listen()
		var bufOut bytes.Buffer
		var bufErr bytes.Buffer

		stdout := make([]Line, 0, 1024)
		stderr := make([]Line, 0, 1024)

		ctx, cancelCtx := context.WithCancel(context.Background())
		for {

			select {
			case <-l.cancelChan:
				cancelListen()
				cancelCtx()
				close(ch)
				return
			case lines := <-inCh:
				for _, line := range lines {
					bufOut.Reset()
					cmd := exec.CommandContext(ctx, l.cmd[0], l.cmd[1:]...)
					cmd.Stdin = bytes.NewReader(line)
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
					stdout = append(stdout, NewLines(bufOut.Bytes())...)
					stderr = append(stderr, NewLines(bufErr.Bytes())...)
					ch <- Output{
						Stdout: stdout,
						Stderr: stderr,
					}
				}
			}
		}
	}()
	return ch
}

func (l *iterative) Close() {
	close(l.cancelChan)
}

var _ Command = (*iterative)(nil)
