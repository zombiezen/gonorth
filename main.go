package main

import (
	"bitbucket.org/zombiezen/gonorth/north"
	"fmt"
	"os"
)

var breakpoints []north.Address
var m *north.Machine

func main() {
	var err error
	m, err = openStory(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Version is:", m.Version())

	for {
		err = debugPrompt()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func debugPrompt() error {
	fmt.Print("\x1b[31m> \x1b[0m")

	var command string
	if _, err := fmt.Scan(&command); err != nil {
		return err
	}

	switch command {
	case "n", "next":
		return m.Step()
	case "b", "break":
		var a north.Address
		if _, err := fmt.Scanf("%x", &a); err != nil {
			return err
		}
		breakpoints = append(breakpoints, a)
	case "c", "cont", "continue":
	continueLoop:
		for {
			err := m.Step()
			if err != nil {
				return err
			}
			for _, bp := range breakpoints {
				if bp == m.PC() {
					break continueLoop
				}
			}
		}
	case "p", "print":
		m.PrintVariables()
	case "w", "word":
		var a north.Address
		if _, err := fmt.Scanf("%x", &a); err != nil {
			return err
		}
		fmt.Println(m.LoadWord(a))
	case "s", "string":
		var a north.Address
		if _, err := fmt.Scanf("%x", &a); err != nil {
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
	// TODO: honor n
	var s string
	_, err := fmt.Scanf("%s", &s)
	return []rune(s), err
}
