package shell

type cat struct {
	cancelChan chan struct{}
}

func NewCat() *cat {
	return &cat{
		cancelChan: make(chan struct{}),
	}
}

func (c *cat) Execute(in Input) <-chan Output {
	ch := make(chan Output)
	go func() {

		inCh, unsubscribe := in.Listen()
		output := Output{
			Stdout: make([]Line, 0, 1024),
			Stderr: nil,
		}
		for {

			select {
			case <-c.cancelChan:
				unsubscribe()
				close(ch)
				return
			case lines := <-inCh:
				output.Stdout = append(output.Stdout, lines...)
				ch <- output
			}
		}
	}()
	return ch
}

func (c *cat) Close() {
	close(c.cancelChan)
}

var _ Command = (*cat)(nil)
