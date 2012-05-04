package main

import (
	"fmt"
	"os"
)

type terminalUI struct { }

func (t *terminalUI) Print(s string) error {
	_, err := fmt.Print(s)
	return err
}

func main() {
	m, err := openStory(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m.ui = new(terminalUI)
	fmt.Println("Version is:", m.Version())

	for {
		err := m.Step()
		if err != nil {
			fmt.Fprintln(os.Stderr, "** Machine:", err)
			os.Exit(1)
		}
	}
}

func openStory(path string) (*Machine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return NewMachine(f)
}
