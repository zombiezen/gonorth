package main

import (
	"bitbucket.org/zombiezen/gonorth/north"
	"fmt"
	"os"
)

func main() {
	m, err := openStory(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Version is:", m.Version())

	for {
		err := m.Step()
		if err != nil {
			fmt.Fprintln(os.Stderr, "** Machine:", err)
			os.Exit(1)
		}
	}
}

func openStory(path string) (*north.Machine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return north.NewMachine(f, new(terminalUI))
}

type terminalUI struct { }

func (t *terminalUI) Print(s string) error {
	_, err := fmt.Print(s)
	return err
}
