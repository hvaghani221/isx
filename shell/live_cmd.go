package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"time"
)

type live struct {
	cmd        []string
	cancelChan chan struct{}
}

func NewLiveCmd(cmd []string) *live {
	return &live{
		cmd:        cmd,
		cancelChan: make(chan struct{}),
	}
}

func (l *live) Execute(in Input) <-chan Output {
	ch := make(chan Output)

	go func() {
		inCh, cancelListen := in.Listen()

		// stdout := make([]Line, 0, 1024)
		// stderr := make([]Line, 0, 1024)

		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()

		cmd := exec.CommandContext(ctx, l.cmd[0], l.cmd[1:]...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			ch <- Output{
				Stdout: nil,
				Stderr: NewLines([]byte(fmt.Errorf("pipe stding: %w", err).Error())),
			}
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer

		checkPoint := [2]int{0, 0}

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			if _, ok := err.(*exec.ExitError); !(ok || errors.Is(err, context.Canceled)) {
				ch <- Output{
					Stdout: nil,
					Stderr: NewLines([]byte(err.Error())),
				}
				log.Printf("error executing command of type %T: %v\n", err, err)
				cancelCtx()
			}
		}

		ticker := time.NewTicker(time.Millisecond * 16)
		for {
			select {
			case <-l.cancelChan:
				cancelListen()
				cancelCtx()
				close(ch)
				ticker.Stop()
				return
			case lines := <-inCh:
				for _, line := range lines {
					_, err := stdin.Write(line)
					if err != nil {
						log.Println("err writing to stdin:", err)
					}
				}
			case <-ticker.C:
				if stdout.Len() > checkPoint[0] || stderr.Len() > checkPoint[1] {
					checkPoint[0] = stdout.Len()
					checkPoint[1] = stderr.Len()
					// TODO: Cache previous NewLines and create lines of updated content
					ch <- Output{
						Stdout: NewLines(stdout.Bytes()),
						Stderr: NewLines(stderr.Bytes()),
					}
				}
			}
		}
	}()
	return ch
}

func (l *live) Close() {
	close(l.cancelChan)
}

var _ Command = (*live)(nil)
