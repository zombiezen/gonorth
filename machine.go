package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
)

// Normal termination by z-machine story.
var (
	ErrQuit    = errors.New("Z-machine quit")
	ErrRestart = errors.New("Z-machine restart")
)

type Address uint16

func (a Address) String() string {
	return fmt.Sprintf("%04x", uint16(a))
}

type Word uint16

func (w Word) String() string {
	return fmt.Sprintf("%#04x", uint16(w))
}

// A stackFrame holds a routine call's data.
type stackFrame struct {
	Return Address
	Locals []Word

	Store         bool
	StoreVariable uint8
}

// At returns the local at 1-based index i.
func (f *stackFrame) At(i int) Word {
	return f.Locals[len(f.Locals)-i]
}

// Set changes the local at 1-based index i.
func (f *stackFrame) Set(i int, x Word) {
	f.Locals[len(f.Locals)-i] = x
}

// PushLocal adds a new local to the local stack.
func (f *stackFrame) PushLocal(x Word) {
	f.Locals = append(f.Locals, x)
}

// PopLocal removes the top local from the stack.
func (f *stackFrame) PopLocal() (x Word) {
	x = f.Locals[len(f.Locals)-1]
	f.Locals = f.Locals[:len(f.Locals)-1]
	return
}

type Machine struct {
	memory []byte
	pc     Address
	stack  []stackFrame
	ui     UI
}

// A UI allows a Machine to interact with a user.
type UI interface {
	//HasStatusLine() bool
	//HasScreenSplitting() bool
	//HasDefaultVariablePitch() bool
	//HasColor() bool

	//ScreenWidth() uint8
	//ScreenHeight() uint8

	Print(string) error
}

// NewMachine creates a new machine, loaded with the story from r.
func NewMachine(r io.Reader) (*Machine, error) {
	m := new(Machine)
	if err := m.Load(r); err != nil {
		return nil, err
	}
	return m, nil
}

// Load starts the machine with a story file in r.
func (m *Machine) Load(r io.Reader) error {
	newMemory, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	m.memory = newMemory
	m.stack = make([]stackFrame, 1)

	// TODO: In version 6+, this is a routine, not a direct PC.
	m.pc = m.initialPC()

	// Debug info
	fmt.Printf("    Version: %d\n", m.Version())
	fmt.Printf("         PC: %v\n", m.initialPC())
	fmt.Printf("Stat Memory: %v\n", m.staticMemoryBase())
	fmt.Printf("High Memory: %v\n", m.highMemoryBase())
	fmt.Printf(" Dictionary: %v\n", m.dictionaryAddress())
	fmt.Printf("  Obj Table: %v\n", m.objectTableAddress())
	fmt.Printf(" Glob Table: %v\n", m.globalVariableTableAddress())
	fmt.Printf("Abbrv Table: %v\n", m.abbreviationTableAddress())
	fmt.Println()

	return nil
}

// memoryReader returns an io.Reader that starts reading at a.
func (m *Machine) memoryReader(a Address) (io.ReadSeeker, error) {
	r := bytes.NewReader(m.memory)
	if _, err := r.Seek(int64(a), 0); err != nil {
		return nil, err
	}
	return r, nil
}

// currStackFrame returns the current stack frame, or nil if the stack is empty.
func (m *Machine) currStackFrame() *stackFrame {
	if len(m.stack) == 0 {
		return nil
	}
	return &m.stack[len(m.stack)-1]
}

// getVariable returns the value of a given variable.
func (m *Machine) getVariable(v uint8) Word {
	switch {
	case v == 0:
		// Pop from stack
		val := m.currStackFrame().PopLocal()
		return val
	case v < 0x10:
		// Local variable
		return m.currStackFrame().At(int(v))
	}
	// Global variable
	return m.loadWord(m.globalVariableTableAddress() + Address((v-0x10)*2))
}

// setVariable changes the value of a given variable.
func (m *Machine) setVariable(v uint8, val Word) {
	switch {
	case v == 0:
		// Push to stack
		m.currStackFrame().PushLocal(val)
	case v < 0x10:
		// Local variable
		m.currStackFrame().Set(int(v), val)
	}
	// Global variable
	m.storeWord(m.globalVariableTableAddress()+Address((v-0x10)*2), val)
}

// fetchOperands returns the values of the operands.
func (m *Machine) fetchOperands(in instruction) []Word {
	ops := make([]Word, in.NOperand())
	for i := range ops {
		val, optype := in.Operand(i)
		switch optype {
		case smallConstantOperand, largeConstantOperand:
			ops[i] = val
		case variableOperand:
			ops[i] = m.getVariable(uint8(val))
		}
	}
	return ops
}

// packedAddress returns the byte address of a packed address.
func (m *Machine) packedAddress(p Word) Address {
	switch m.Version() {
	case 1, 2, 3:
		return Address(2 * p)
	case 4, 5:
		return Address(4 * p)
	// TODO: 6, 7
	case 8:
		return Address(8 * p)
	}
	panic("Bad machine version for packed address!!")
}

// Version returns the version of the machine, defined in the story file.
func (m *Machine) Version() byte {
	return m.memory[0]
}

func (m *Machine) loadWord(a Address) Word {
	return Word(m.memory[a])<<8 | Word(m.memory[a+1])
}

func (m *Machine) storeWord(a Address, w Word) {
	m.memory[a] = byte(w >> 8)
	m.memory[a+1] = byte(w & 0x00ff)
}

// loadString decodes a ZSCII string at address addr.  See NewZSCIIDecoder for
// the output parameter.
func (m *Machine) loadString(addr Address, output bool) (string, error) {
	r, err := m.memoryReader(addr)
	if err != nil {
		return "", err
	}
	// TODO: alphabet set
	return decodeString(r, StandardAlphabetSet, output, m)
}

func (m *Machine) Unabbreviate(entry int) (string, error) {
	entryWord := m.loadWord(m.abbreviationTableAddress() + Address(entry*2))
	r, err := m.memoryReader(Address(entryWord) * 2)
	if err != nil {
		return "", err
	}
	// TODO: alphabet set
	// TODO: output?
	return decodeString(r, StandardAlphabetSet, true, nil)
}

func (m *Machine) initialPC() Address {
	return Address(m.loadWord(0x6))
}

func (m *Machine) highMemoryBase() Address {
	return Address(m.loadWord(0x4))
}

func (m *Machine) dictionaryAddress() Address {
	return Address(m.loadWord(0x8))
}

func (m *Machine) objectTableAddress() Address {
	return Address(m.loadWord(0xa))
}

func (m *Machine) globalVariableTableAddress() Address {
	return Address(m.loadWord(0xc))
}

func (m *Machine) staticMemoryBase() Address {
	return Address(m.loadWord(0xe))
}

func (m *Machine) abbreviationTableAddress() Address {
	return Address(m.loadWord(0x18))
}
