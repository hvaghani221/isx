package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sync"
	"time"
)

const (
	tickInterval = time.Millisecond * 16
)

type Line []byte

type PubSub struct {
	mu          sync.Mutex
	data        []Line
	subscribers map[int]*subscrption
	nextID      int
}

type subscrption struct {
	ch    chan []Line
	index int
}

func NewPubSub() *PubSub {
	return &PubSub{
		subscribers: make(map[int]*subscrption),
		data:        make([]Line, 0, 1024),
	}
}

func (s *PubSub) Subscribe() (<-chan []Line, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++

	ch := make(chan []Line)
	s.subscribers[id] = &subscrption{
		ch:    ch,
		index: 0,
	}

	return ch, func() {
		log.Println("cancel subscription: ", id)
		s.mu.Lock()
		defer s.mu.Unlock()
		close(ch)
		delete(s.subscribers, id)
	}
}

func (s *PubSub) Publish(ctx context.Context, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.Println("publish: ", countLines(data))
	s.data = append(s.data, data)
}

func (s *PubSub) publishLoop(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	prevCancel := func() {}
	for {
		select {
		case <-ctx.Done():
			prevCancel()
			return
		case <-ticker.C:
			prevCancel()
			s.mu.Lock()
			nLine := len(s.data)
			s.mu.Unlock()

			var innerContext context.Context
			innerContext, prevCancel = context.WithCancel(ctx)
			for _, sub := range s.subscribers {
				sub := sub
				if sub.index == nLine {
					continue
				}
				go func() {
					select {
					case sub.ch <- s.data[sub.index:nLine]:
						sub.index = nLine
					case <-innerContext.Done():
						return
					}
				}()
			}
		}
	}
}

type shell struct {
	in         io.ReadCloser
	pubsub     *PubSub
	cancelFunc func()
}

func NewShell(in io.ReadCloser) *shell {
	return &shell{
		in:     in,
		pubsub: NewPubSub(),
	}
}

func (s *shell) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		s.in.Close()
	}()
	go s.pubsub.publishLoop(ctx)
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := s.in.Read(buffer)
			if err != nil {
				if n > 0 {
					panic("EOF with unread bytes")
				}
				if err != io.EOF || errors.Is(err, context.Canceled) {
					panic(err)
				}
				return
			}
			tmpBuffer := buffer[:n]
			for i := bytes.IndexByte(tmpBuffer, '\n'); i >= 0; i = bytes.IndexByte(tmpBuffer, '\n') {
				line := make([]byte, i+1)
				copy(line, tmpBuffer[:i+1])
				s.pubsub.Publish(ctx, line)
				tmpBuffer = tmpBuffer[i+1:]
			}
			if len(tmpBuffer) > 0 {
				s.pubsub.Publish(ctx, tmpBuffer)
			}
		}
	}()
}

func (s *shell) ListenInput(ctx context.Context) <-chan []byte {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	ctx, s.cancelFunc = context.WithCancel(ctx)

	inCh, unsubscribe := s.pubsub.Subscribe()
	ch := make(chan []byte)
	go func() {
		for {
			select {
			case <-ctx.Done():
				unsubscribe()
				close(ch)
				s.cancelFunc = nil
				return
			case lines := <-inCh:
				for _, line := range lines {
					ch <- line
				}
			}
		}
	}()
	return ch
}

func (s *shell) ExecuteCommandLineByLine(ctx context.Context, cmd string) <-chan []byte {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	ctx, s.cancelFunc = context.WithCancel(ctx)

	inCh, unsubscribe := s.pubsub.Subscribe()

	ch := make(chan []byte)
	cmds := parseCommand(cmd)
	go func() {
		var buffer bytes.Buffer
		for {
			select {
			case <-ctx.Done():
				unsubscribe()
				close(ch)
				s.cancelFunc = nil
				log.Println("Finish executing command: ", cmd)
				log.Print(buffer.String())
				return
			case lines := <-inCh:
				for _, line := range lines {
					buffer.Reset()
					cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
					cmd.Stdin = bytes.NewReader(line)
					cmd.Stdout = &buffer
					cmd.Stderr = &buffer

					if err := cmd.Run(); err != nil {
						if _, ok := err.(*exec.ExitError); !(ok || errors.Is(err, context.Canceled)) {
							ch <- []byte(err.Error())
							log.Printf("error executing command of type %T: %v\n", err, err)
							continue
						}
					}
					result := make([]byte, buffer.Len())
					copy(result, buffer.Bytes())
					ch <- result
				}
			}
		}
	}()
	return ch
}

func (s *shell) ExecuteCommand(ctx context.Context, cmd string) <-chan []byte {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	ctx, s.cancelFunc = context.WithCancel(ctx)

	inCh, unsubscribe := s.pubsub.Subscribe()

	ch := make(chan []byte)
	cmds := parseCommand(cmd)
	go func() {
		lineBuffer := make([]Line, 0, 1024)
		var buffer bytes.Buffer
		for {
			select {
			case <-ctx.Done():
				unsubscribe()
				close(ch)
				s.cancelFunc = nil
				return
			case lines := <-inCh:
				buffer.Reset()
				lineBuffer = append(lineBuffer, lines...)
				cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
				cmd.Stdin = NewMultilineReader(lineBuffer)
				cmd.Stdout = &buffer
				cmd.Stderr = &buffer

				if err := cmd.Run(); err != nil {
					if _, ok := err.(*exec.ExitError); !(ok || errors.Is(err, context.Canceled)) {
						ch <- []byte(err.Error())
						log.Printf("error executing command of type %T: %v\n", err, err)
						continue
					}
				}
				result := make([]byte, buffer.Len())
				copy(result, buffer.Bytes())
				ch <- result
			}
		}
	}()
	return ch
}

func countLines(data []byte) int {
	return bytes.Count(data, []byte{'\n'})
}

var pattern = regexp.MustCompile(`"[^"]*"|'[^']*'|\S+`)

func parseCommand(cmd string) []string {
	return pattern.FindAllString(cmd, -1)
}

func Main1() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	shell := NewShell(os.Stdin)
	shell.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		res := shell.ExecuteCommandLineByLine(ctx, "wc -c")
		for line := range res {
			fmt.Print(string(line))
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		res := shell.ExecuteCommandLineByLine(ctx, "wc")
		for line := range res {
			fmt.Print(string(line))
		}
	}()

	wg.Wait()
}

func Main() {
	cmd := exec.Command("go", "--help", "go --help")

	// Creating pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("stdoutPipe err: ", err)
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("stderrPipe err: ", err)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Println("start err: ", err)
		return
	}

	// Use goroutines to copy the output in real-time
	go io.Copy(os.Stdout, stdoutPipe)
	go io.Copy(os.Stderr, stderrPipe)

	time.Sleep(time.Second)
	fmt.Println("sleep finish")

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		fmt.Println("wait err: ", err)
	}
}
