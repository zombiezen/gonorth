package main

import (
	"bitbucket.org/zombiezen/gonorth/north"
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
)

var breakpoints []north.Address
var m *north.Machine
var in *bufio.Reader

func main() {
	in = bufio.NewReader(os.Stdin)

	debug := flag.Bool("debug", false, "Run story in debugger")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("usage: gonorth [options] FILE")
		os.Exit(2)
	}

	var err error
	m, err = openStory(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !*debug {
		for {
			err = m.Step()
			switch err {
			case nil:
			case io.EOF, north.ErrQuit:
				os.Exit(0)
			case north.ErrRestart:
				m, err = openStory(flag.Arg(0))
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			default:
				fmt.Fprintln(os.Stderr, "** Internal Error:", err)
				os.Exit(1)
			}
		}
	} else {
		fmt.Println("Version is:", m.Version())
		for {
			err = debugPrompt()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
}

func debugPrompt() error {
	fmt.Print("\x1b[31m> \x1b[0m")

	var command string
	if _, err := fmt.Fscan(in, &command); err != nil {
		return err
	}

	switch command {
	case "n", "next":
		in.ReadLine()
		return m.Step()
	case "b", "break":
		var a north.Address
		if _, err := fmt.Fscanf(in, "%x", &a); err != nil {
			return err
		}
		breakpoints = append(breakpoints, a)
	case "c", "cont", "continue":
		in.ReadLine()
		for {
			err := m.Step()
			if err != nil {
				return err
			}
			for _, bp := range breakpoints {
				if bp == m.PC() {
					return nil
				}
			}
		}
	case "p", "print":
		m.PrintVariables()
	case "v", "var", "variable":
		var v uint8
		if _, err := fmt.Fscanf(in, "%x", &v); err != nil {
			return err
		}
		fmt.Printf("$%02x: %v\n", v, m.Variable(v))
	case "w", "word":
		var a north.Address
		if _, err := fmt.Fscanf(in, "%x", &a); err != nil {
			return err
		}
		fmt.Println(m.LoadWord(a))
	case "s", "string":
		var a north.Address
		if _, err := fmt.Fscanf(in, "%x", &a); err != nil {
			return err
		}
		if s, err := m.LoadString(a); err == nil {
			fmt.Printf("%v: %q\n", a, s)
		} else {
			fmt.Println("Decode error:", err)
		}
	case "q", "quit", "exit":
		os.Exit(0)
	default:
		fmt.Println("Bad command:", command)
	}

	return nil
}

func openStory(path string) (*north.Machine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return north.NewMachine(f, new(terminalUI))
}

type terminalUI struct{}

func (t *terminalUI) Print(s string) error {
	_, err := fmt.Print(s)
	return err
}

func (t *terminalUI) Read(n int) ([]rune, error) {
	r := make([]rune, 0, n)
	for {
		rr, _, err := in.ReadRune()
		if err != nil {
			return r, err
		} else if rr == '\n' {
			break
		}
		if len(r) < n {
			r = append(r, rr)
		}
	}
	return r, nil
}
