package shell

type Mode int

const (
	Snapshot Mode = iota
	Live
	Iterative
)
