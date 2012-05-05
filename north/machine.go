package north

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
	Stack  []Word

	Store         bool
	StoreVariable uint8
}

// LocalAt returns the local at 1-based index i.
func (f *stackFrame) LocalAt(i int) Word {
	return f.Locals[i-1]
}

// SetLocal changes the local at 1-based index i.
func (f *stackFrame) SetLocal(i int, x Word) {
	f.Locals[i-1] = x
}

// Push adds a new value to the stack.
func (f *stackFrame) Push(w Word) {
	f.Stack = append(f.Stack, w)
}

// Pop removes the top value from the stack.
func (f *stackFrame) Pop() (w Word) {
	w = f.Stack[len(f.Stack)-1]
	f.Stack = f.Stack[:len(f.Stack)-1]
	return
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

type Machine struct {
	memory []byte
	pc     Address
	stack  []stackFrame
	ui     UI
}

// NewMachine creates a new machine, loaded with the story from r.
func NewMachine(r io.Reader, ui UI) (*Machine, error) {
	m := new(Machine)
	if err := m.Load(r); err != nil {
		return nil, err
	}
	m.SetUI(ui)
	return m, nil
}

// UI returns m's user interface.
func (m *Machine) UI() UI {
	return m.ui
}

// SetUI sets m's user interface.
func (m *Machine) SetUI(ui UI) {
	m.ui = ui
	if m.memory != nil {
		m.copyUIFlags()
	}
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

	// Standard revision number
	// XXX: Change to 0x0100 when compliant
	m.storeWord(0x32, 0x0000)

	m.copyUIFlags()

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

func (m *Machine) copyUIFlags() {
	if m.Version() > 3 {
		// TODO
		return
	}

	m.memory[1] &^= 0x70
	// No status line (yet)
	m.memory[1] |= 0x10
}

// PC returns the program counter.
func (m *Machine) PC() Address {
	return m.pc
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

func (m *Machine) PrintVariables() {
	fmt.Printf("PC:  %v\n", m.pc)
	for i, val := range m.currStackFrame().Locals {
		fmt.Printf("$%02x: %v\n", i+1, val)
	}
	for i, val := range m.currStackFrame().Stack {
		fmt.Printf("S%2d: %v\n", i, val)
	}
}

func (m *Machine) LoadWord(a Address) Word {
	return m.loadWord(a)
}

func (m *Machine) LoadString(a Address) (string, error) {
	return m.loadString(a, true)
}

// globalAddress returns of g (a 0-based index into the global table).
func (m *Machine) globalAddress(g uint8) Address {
	return m.globalVariableTableAddress() + Address(g)*2
}

// getVariable returns the value of a given variable.
func (m *Machine) getVariable(v uint8) Word {
	switch {
	case v == 0:
		// Pop from stack
		return m.currStackFrame().Pop()
	case v < 0x10:
		// Local variable
		return m.currStackFrame().LocalAt(int(v))
	}
	// Global variable
	return m.loadWord(m.globalAddress(v - 0x10))
}

// setVariable changes the value of a given variable.
func (m *Machine) setVariable(v uint8, val Word) {
	switch {
	case v == 0:
		// Push to stack
		m.currStackFrame().Push(val)
	case v < 0x10:
		// Local variable
		m.currStackFrame().SetLocal(int(v), val)
	}
	// Global variable
	m.storeWord(m.globalAddress(v-0x10), val)
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
