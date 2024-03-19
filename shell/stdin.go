package shell

import (
	"bytes"
	"io"
	"sync"
	"time"
)

type input struct {
	mu          sync.Mutex
	data        []Line
	subscribers map[int]*subscrption
	nextID      int
	in          io.ReadCloser
	cancelChan  chan struct{}

	incompleteLine []byte
}

type subscrption struct {
	ch    chan []Line
	index int
}

func NewInput(in io.ReadCloser) *input {
	input := &input{
		subscribers: make(map[int]*subscrption),
		data:        make([]Line, 0, 1024),
		in:          in,
		cancelChan:  make(chan struct{}),
	}

	input.start()
	return input
}

func (in *input) Listen() (<-chan []Line, func()) {
	in.mu.Lock()
	defer in.mu.Unlock()
	id := in.nextID
	in.nextID++

	ch := make(chan []Line)
	in.subscribers[id] = &subscrption{
		ch:    ch,
		index: 0,
	}

	return ch, func() {
		in.mu.Lock()
		defer in.mu.Unlock()
		close(ch)
		delete(in.subscribers, id)
	}
}

func (in *input) Close() {
	in.in.Close()
	close(in.cancelChan)
}

func (in *input) start() {
	go in.publishLoop()
	go func() {
		buffer := make([]byte, 128)
		for {
			n, err := in.in.Read(buffer)
			if err != nil {
				if n > 0 {
					panic("EOF with unread bytes")
				}
				if err == io.EOF {
					in.closeInput()
					return
				}
				panic(err)
				// return
			}
			tmpBuffer := buffer[:n]
			for i := bytes.IndexByte(tmpBuffer, '\n'); i >= 0; i = bytes.IndexByte(tmpBuffer, '\n') {
				in.publish(tmpBuffer[:i+1])
				tmpBuffer = tmpBuffer[i+1:]
			}
			if len(tmpBuffer) > 0 {
				in.publish(tmpBuffer)
			}
		}
	}()
}

func (in *input) publish(data []byte) {
	in.mu.Lock()
	defer in.mu.Unlock()

	in.incompleteLine = append(in.incompleteLine, data...)

	if in.incompleteLine[len(in.incompleteLine)-1] == '\n' {
		in.data = append(in.data, in.incompleteLine)
		in.incompleteLine = nil
	}

}

func (in *input) publishLoop() {
	ticker := time.NewTicker(tickInterval)
	for {
		select {
		case <-in.cancelChan:
			return
		case <-ticker.C:
			in.mu.Lock()
			nLine := len(in.data)
			in.mu.Unlock()

			for _, sub := range in.subscribers {
				sub := sub
				if sub.index == nLine {
					continue
				}
				go func() {
					select {
					case sub.ch <- in.data[sub.index:nLine]:
						sub.index = nLine
					case <-in.cancelChan:
						return
					}
				}()
			}
		}
	}
}

func (in *input) closeInput() {
	in.mu.Lock()
	defer in.mu.Unlock()

	if len(in.incompleteLine) > 0 {
		in.data = append(in.data, in.incompleteLine)
		in.incompleteLine = nil
	}
}
