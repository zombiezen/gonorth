package north

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"time"
)

// Normal termination by z-machine story.
var (
	ErrQuit    = errors.New("Z-machine quit")
	ErrRestart = errors.New("Z-machine restart")
)

type Address int

func (a Address) String() string {
	return fmt.Sprintf("%05x", uint(a))
}

type Word uint16

func (w Word) String() string {
	return fmt.Sprintf("%#04x", uint16(w))
}

// A stackFrame holds a routine call's data.
type stackFrame struct {
	PC     Address
	Locals []Word
	Stack  []Word

	Store         bool
	StoreVariable uint8

	NArg uint8
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
	io.RuneReader
	Input(n int) ([]rune, error)
	Output(window int, text string) error
	Save(m *Machine) error
	Restore(m *Machine) error
}

// StatusLiner is a UI that can display a status line.
type StatusLiner interface {
	StatusLine(left, right string) error
}

// Predefined sound effects
const (
	HighPitchBleep = 1
	LowPitchBleep  = 2
)

// SoundPlayer is a UI that can play sounds.
type SoundPlayer interface {
	PrepareSound(n int) error
	PlaySound(n int, volume int8, repeats uint8) error
	StopSound(n int) error
	FinishSound(n int) error
}

// Output streams
const (
	screenOutput = 1 + iota
	transcriptOutput
	redirectOutput
	readOutput

	numOutputStreams
)

// rtable is a redirect table pointer.
type rtable struct {
	Start Address
	Curr  Address
}

type Machine struct {
	memory []byte
	stack  []stackFrame
	ui     UI
	rand   *rand.Rand

	window  int
	streams uint8
	rtables []rtable
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

// Run executes the story until an error occurs.
func (m *Machine) Run() error {
	for {
		err := m.Step()
		if err != nil {
			return err
		}
	}
	panic("never reached")
}

// Load starts the machine with a story file in r.
func (m *Machine) Load(r io.Reader) error {
	newMemory, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	m.memory = newMemory
	m.stack = make([]stackFrame, 1)
	m.rtables = make([]rtable, 0, 16)
	m.streams = 1<<screenOutput | 1<<transcriptOutput
	m.seed()

	// TODO: In version 6+, this is a routine, not a direct PC.
	m.stack[0].PC = m.initialPC()

	// Standard revision number
	// XXX: Change to 0x0100 when compliant
	m.storeWord(0x32, 0x0000)

	m.copyUIFlags()

	return nil
}

// SaveStack encodes the stack to w.
func (m *Machine) SaveStack(w io.Writer) error {
	e := gob.NewEncoder(w)
	return e.Encode(m.stack)
}

// RestoreStack decodes the stack from r.
func (m *Machine) RestoreStack(r io.Reader) error {
	d := gob.NewDecoder(r)
	return d.Decode(&m.stack)
}

func (m *Machine) copyUIFlags() {
	const (
		flags1       Address = 0x01
		flags2       Address = 0x10
		screenWidth  Address = 0x20
		screenHeight Address = 0x21
	)

	if m.Version() < 4 {
		m.memory[flags1] &= 0x8f
		if _, ok := m.ui.(StatusLiner); !ok {
			m.memory[flags1] |= 1 << 4
		}
		return
	}

	m.memory[flags1] &= 0x40
	if _, ok := m.ui.(SoundPlayer); ok {
		m.memory[flags1] |= 1 << 5
	}
	m.memory[flags2] &= 0x47
	if _, ok := m.ui.(SoundPlayer); ok {
		m.memory[flags2] |= 1 << 7
	}
	// TODO
	m.memory[screenWidth] = 255
	m.memory[screenHeight] = 255
}

// out handles output. This is sent to the UI, unless redirection has been
// turned on.
func (m *Machine) out(s string) error {
	if m.streams&(1<<redirectOutput) != 0 {
		// If redirect is selected, no other streams get output.
		tab := &m.rtables[len(m.rtables)-1]
		m.storeWord(tab.Start, m.loadWord(tab.Start)+Word(len(s)))
		for _, r := range s {
			// rune should already be ZSCII-clean, since we wrote it.
			m.memory[tab.Curr] = byte(r)
			tab.Curr++
		}
		return nil
	}
	if m.streams&(1<<screenOutput) != 0 {
		if err := m.ui.Output(m.window, s); err != nil {
			return err
		}
	}
	// TODO: transcript, etc.
	return nil
}

func (m *Machine) refreshStatusLine() error {
	liner, ok := m.ui.(StatusLiner)
	if !ok {
		return nil
	}

	isTime := m.memory[1]&0x02 != 0
	name, err := m.loadObject(m.getVariable(0x10)).FetchName(m)
	if err != nil {
		return err
	}

	var right string
	if isTime {
		h, m := int16(m.getVariable(0x11)), int16(m.getVariable(0x12))
		switch {
		case h == 0:
			right = fmt.Sprintf("12:%02d AM", m)
		case h < 12:
			right = fmt.Sprintf("%2d:%02d AM", h, m)
		case h == 12:
			right = fmt.Sprintf("12:%02d PM", m)
		default:
			right = fmt.Sprintf("%2d:%02d PM", h-12, m)
		}
	} else {
		right = fmt.Sprintf("%3d/%4d", int16(m.getVariable(0x11)), int16(m.getVariable(0x12)))
	}

	return liner.StatusLine(name, right)
}

// PC returns the program counter.
func (m *Machine) PC() Address {
	return m.currStackFrame().PC
}

// MemoryReader returns an io.Reader that starts reading at a.
func (m *Machine) MemoryReader(a Address) (io.ReadSeeker, error) {
	r := bytes.NewReader(m.memory)
	if _, err := r.Seek(int64(a), 0); err != nil {
		return nil, err
	}
	return r, nil
}

// currStackFrame returns the current stack frame, or nil if the stack is empty.
func (m *Machine) currStackFrame() *stackFrame {
	return &m.stack[len(m.stack)-1]
}

func (m *Machine) PrintVariables() {
	fmt.Printf("PC:  %v\n", m.currStackFrame().PC)
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

func (m *Machine) Variable(v uint8) Word {
	if v == 0 {
		return 0
	}
	return m.getVariable(v)
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
	default:
		// Global variable
		m.storeWord(m.globalAddress(v-0x10), val)
	}
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
		return 2 * Address(p)
	case 4, 5:
		return 4 * Address(p)
	// TODO: 6, 7
	case 8:
		return 8 * Address(p)
	}
	panic("Bad machine version for packed address!!")
}

// Version returns the version of the machine, defined in the story file.
func (m *Machine) Version() byte {
	return m.memory[0]
}

// seed restarts the random generator with the current time as a seed.
func (m *Machine) seed() {
	m.rand = rand.New(rand.NewSource(time.Now().Unix()))
}

// random returns the next random number.
func (m *Machine) random(s Word) Word {
	return Word(m.rand.Uint32()%uint32(s) + 1)
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
	r, err := m.MemoryReader(addr)
	if err != nil {
		return "", err
	}
	// TODO: alphabet set
	return decodeString(r, StandardAlphabetSet, output, m)
}

func (m *Machine) Unabbreviate(entry int) (string, error) {
	entryWord := m.loadWord(m.abbreviationTableAddress() + Address(entry)*2)
	r, err := m.MemoryReader(Address(entryWord) * 2)
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
