package main

import (
	"fmt"
	"os"
	"runtime/pprof"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hvaghani221/isx/shell"
	"github.com/hvaghani221/isx/ui"
)

func main() {
	profile, err := os.Create("profile.pprof")
	if err != nil {
		panic(err)
	}

	defer profile.Close()

	_ = pprof.StartCPUProfile(profile)
	defer pprof.StopCPUProfile()

	fstat, err := os.Stdin.Stat()
	if err != nil {
		panic(err)
	}

	if fstat.Mode()&os.ModeCharDevice != 0 {
		fmt.Println("stdin should be piped")
		os.Exit(1)
	}

	file, err := tea.LogToFile("debug.log", "")
	if err != nil {
		panic(err)
	}
	_ = file.Truncate(0)
	defer file.Close()

	stdout := os.Stdout
	writeToStdout := false
	o, _ := os.Stdout.Stat()
	if (o.Mode() & os.ModeCharDevice) != os.ModeCharDevice {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			panic(err) // handle error properly in production code
		}
		defer tty.Close()

		writeToStdout = true
		os.Stdout = tty
	}

	in := shell.NewInput(os.Stdin)

	m, program := ui.NewProgram(in)
	if _, err := program.Run(); err != nil {
		panic(err)
	}

	if writeToStdout {
		m.WriteOutputTo(stdout)
	}

}
