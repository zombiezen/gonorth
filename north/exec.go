package north

import (
	"errors"
	"fmt"
	"unicode"
)

// Step executes the next opcode in the machine.
func (m *Machine) Step() error {
	r, err := m.memoryReader(m.PC())
	if err != nil {
		return err
	}
	// TODO: Get story alphabet set
	i, err := decodeInstruction(r, StandardAlphabetSet, m)
	if err != nil {
		return err
	}
	fmt.Printf("\x1b[34m%v\x1b[33m\t%v\x1b[0m\n", m.PC(), i)
	newPC, _ := r.Seek(0, 1)
	m.currStackFrame().PC = Address(newPC)

	switch in := i.(type) {
	case *longInstruction:
		return m.step2OPInstruction(in)
	case *shortInstruction:
		if in.NOperand() == 0 {
			return m.step0OPInstruction(in)
		} else {
			return m.step1OPInstruction(in)
		}
	case *variableInstruction:
		if in.is2OP() {
			return m.step2OPInstruction(in)
		} else {
			return m.stepVariableInstruction(in)
		}
	}
	return errors.New("Instruction type not implemented yet")
}

func (m *Machine) routineCall(address Address, args []Word, ret uint8) error {
	if address == 0 {
		m.setVariable(ret, 0)
		return nil
	}
	nlocals := int(m.memory[address])
	if nlocals > 15 {
		return errors.New("Routines have a maximum of 15 local variables")
	}
	newFrame := stackFrame{
		PC:            address + 1,
		Locals:        make([]Word, nlocals),
		Store:         true,
		StoreVariable: ret,
	}
	if m.Version() <= 4 {
		for i := range newFrame.Locals {
			newFrame.Locals[i] = m.loadWord(address + 1 + Address(i)*2)
		}
		newFrame.PC += Address(nlocals) * 2
	}
	copy(newFrame.Locals, args)
	m.stack = append(m.stack, newFrame)
	return nil
}

func (m *Machine) routineNCall(address Address, args []Word) error {
	// TODO
	return errors.New("routineNCall not implemented yet")
}

func (m *Machine) routineReturn(val Word) error {
	if len(m.stack) == 1 {
		return errors.New("return from main")
	}

	frame := m.currStackFrame()
	m.stack = m.stack[:len(m.stack)-1]
	if frame.Store {
		m.setVariable(frame.StoreVariable, val)
	}
	return nil
}

func (m *Machine) conditional(branch branchInfo, test bool) error {
	if test == branch.Condition() {
		switch branch.Offset() {
		case 0:
			return m.routineReturn(0)
		case 1:
			return m.routineReturn(1)
		default:
			m.currStackFrame().PC += Address(branch.Offset()) - 2
		}
	}
	return nil
}

func (m *Machine) step2OPInstruction(in instruction) error {
	ops := m.fetchOperands(in)
	branch, _ := in.BranchInfo()
	storeVariable, _ := in.StoreVariable()
	switch in.OpcodeNumber() {
	case 0x01:
		// je
		var eq bool
		for i := 1; i < len(ops); i++ {
			if ops[0] == ops[i] {
				eq = true
				break
			}
		}
		return m.conditional(branch, eq)
	case 0x02:
		// jl
		return m.conditional(branch, int16(ops[0]) < int16(ops[1]))
	case 0x03:
		// jg
		return m.conditional(branch, int16(ops[0]) > int16(ops[1]))
	case 0x04:
		// dec_chk
		newVal := m.getVariable(uint8(ops[0])) - 1
		m.setVariable(uint8(ops[0]), newVal)
		return m.conditional(branch, int16(newVal) < int16(ops[1]))
	case 0x05:
		// inc_chk
		newVal := m.getVariable(uint8(ops[0])) + 1
		m.setVariable(uint8(ops[0]), newVal)
		return m.conditional(branch, int16(newVal) > int16(ops[1]))
	case 0x06:
		// jin
		obj1 := m.loadObject(ops[0])
		return m.conditional(branch, obj1.Parent == ops[1])
	case 0x07:
		// test
		return m.conditional(branch, ops[0]&ops[1] == ops[1])
	case 0x08:
		// or
		m.setVariable(storeVariable, ops[0]|ops[1])
	case 0x09:
		// and
		m.setVariable(storeVariable, ops[0]&ops[1])
	case 0x0a:
		// test_attr
		obj := m.loadObject(ops[0])
		return m.conditional(branch, obj.Attr(uint8(ops[1])))
	case 0x0b:
		// set_attr
		obj := m.loadObject(ops[0])
		obj.SetAttr(uint8(ops[1]), true)
		m.storeObject(ops[0], obj)
	case 0x0d:
		// store
		m.setVariable(uint8(ops[0]), ops[1])
	case 0x0e:
		// insert_obj
		o, d := m.loadObject(ops[0]), m.loadObject(ops[1])
		// TODO: what if o.parent != 0?
		o.Sibling = d.Child
		o.Parent = ops[1]
		d.Child = ops[0]
		m.storeObject(ops[0], o)
		m.storeObject(ops[1], d)
	case 0x0f:
		// loadw
		a := Address(ops[0]) + 2*Address(ops[1])
		m.setVariable(storeVariable, m.loadWord(a))
	case 0x10:
		// loadb
		// TODO: should the value be sign extended?
		a := Address(ops[0]) + Address(ops[1])
		m.setVariable(storeVariable, Word(m.memory[a]))
	case 0x11:
		// get_prop
		obj := m.loadObject(ops[0])
		p := obj.Property(m, uint8(ops[1]))
		switch len(p) {
		case 0:
			m.setVariable(storeVariable, m.defaultPropertyValue(uint8(ops[1])))
		case 1:
			m.setVariable(storeVariable, Word(p[0]))
		case 2:
			m.setVariable(storeVariable, Word(p[0])<<8|Word(p[1]))
		default:
			return fmt.Errorf("Mismatched property size: vs. %d", len(p))
		}
	case 0x12:
		// get_prop_addr
		obj := m.loadObject(ops[0])
		m.setVariable(storeVariable, Word(obj.PropertyAddress(m, uint8(ops[1]))))
	case 0x14:
		// add
		m.setVariable(storeVariable, Word(int16(ops[0])+int16(ops[1])))
	case 0x15:
		// sub
		m.setVariable(storeVariable, Word(int16(ops[0])-int16(ops[1])))
	case 0x16:
		// mul
		m.setVariable(storeVariable, Word(int16(ops[0])*int16(ops[1])))
	case 0x17:
		// div
		m.setVariable(storeVariable, Word(int16(ops[0])/int16(ops[1])))
	case 0x18:
		// mod
		m.setVariable(storeVariable, Word(int16(ops[0])%int16(ops[1])))
	default:
		return errors.New("2OP opcode not implemented yet")
	}
	return nil
}

func (m *Machine) step1OPInstruction(in *shortInstruction) error {
	ops := m.fetchOperands(in)
	switch in.OpcodeNumber() {
	case 0x0:
		// jz
		return m.conditional(in.branch, ops[0] == 0)
	case 0x1:
		// get_sibling
		obj := m.loadObject(ops[0])
		m.setVariable(in.storeVariable, obj.Sibling)
		return m.conditional(in.branch, obj.Sibling != 0)
	case 0x2:
		// get_child
		obj := m.loadObject(ops[0])
		m.setVariable(in.storeVariable, obj.Child)
		return m.conditional(in.branch, obj.Child != 0)
	case 0x3:
		// get_parent
		obj := m.loadObject(ops[0])
		m.setVariable(in.storeVariable, obj.Parent)
	case 0x5:
		// inc
		m.setVariable(uint8(ops[0]), m.getVariable(uint8(ops[0]))+1)
	case 0x6:
		// dec
		m.setVariable(uint8(ops[0]), m.getVariable(uint8(ops[0]))-1)
	case 0x7:
		// print_addr
		s, err := m.loadString(Address(ops[0]), true)
		if err != nil {
			return err
		}
		return m.ui.Print(s)
	case 0xa:
		// print_obj
		obj := m.loadObject(ops[0])
		// TODO: check obj for nil
		s, err := obj.FetchName(m)
		if err != nil {
			return err
		}
		return m.ui.Print(s)
	case 0xb:
		// ret
		return m.routineReturn(ops[0])
	case 0xc:
		// jump
		// TODO: do we ever use 0 or 1 offsets here?
		m.currStackFrame().PC += Address(int16(ops[0])) - 2
	default:
		return errors.New("1OP opcode not implemented yet")
	}
	return nil
}

func (m *Machine) step0OPInstruction(in *shortInstruction) error {
	switch in.OpcodeNumber() {
	case 0x0:
		// rtrue
		return m.routineReturn(1)
	case 0x1:
		// rfalse
		return m.routineReturn(0)
	case 0x2:
		// print
		return m.ui.Print(in.text)
	case 0x4:
		// nop
	case 0x7:
		// restart
		return ErrRestart
	case 0x8:
		// ret_popped
		m.routineReturn(m.currStackFrame().Pop())
	case 0xa:
		// quit
		return ErrQuit
	case 0xb:
		// new_line
		return m.ui.Print("\n")
	case 0xc:
		// show_status
		// This acts as a nop
	default:
		return errors.New("0OP opcode not implemented yet")
	}
	return nil
}

func (m *Machine) stepVariableInstruction(in *variableInstruction) error {
	ops := m.fetchOperands(in)
	switch in.OpcodeNumber() {
	case 0x0:
		// call (v3), call_vs (v4+)
		if ops[0] == 0 {
			return m.routineCall(0, nil, in.storeVariable)
		} else {
			return m.routineCall(m.packedAddress(ops[0]), ops[1:], in.storeVariable)
		}
	case 0x1:
		// storew
		a := Address(ops[0]) + 2*Address(ops[1])
		m.storeWord(a, ops[2])
	case 0x2:
		// storeb
		a := Address(ops[0]) + 2*Address(ops[1])
		m.memory[a] = byte(ops[2])
	case 0x3:
		// put_prop
		obj := m.loadObject(ops[0])
		p := obj.Property(m, uint8(ops[1]))
		switch len(p) {
		case 1:
			p[0] = byte(ops[2] & 0xff)
		case 2:
			p[0] = byte(ops[2] >> 8)
			p[1] = byte(ops[2] & 0xff)
		default:
			return fmt.Errorf("Mismatched property size: vs. %d", len(p))
		}
	case 0x4:
		// read
		// TODO: Versions 1-3 redisplay status line
		var input []rune
		if m.Version() <= 4 {
			var err error
			input, err = m.ui.Read(int(m.memory[Address(ops[0])]) + 1)
			if err != nil {
				return err
			}

			for i := range input {
				// TODO: Ensure input is ZSCII-clean
				input[i] = unicode.ToLower(input[i])
				m.memory[Address(ops[0])+1+Address(i)] = byte(input[i])
			}
			m.memory[Address(ops[0])+1+Address(len(input))] = 0
		} else {
			// TODO
			return errors.New("Read not implemented for version 5+")
		}

		if m.Version() < 5 || ops[1] != 0 {
			dict, err := m.dictionary()
			if err != nil {
				return err
			}
			words := lex(input, dict)
			maxWords := m.memory[ops[1]]
			if len(words) > int(maxWords) {
				words = words[:maxWords]
			}
			m.memory[Address(ops[1])+1] = byte(len(words))
			base := Address(ops[1]) + 2
			for i := range words {
				m.storeWord(base+Address(i)*4, Word(words[i].Word))
				m.memory[base+Address(i)*4+2] = byte(words[i].Start)
				m.memory[base+Address(i)*4+3] = byte(words[i].End)
			}
		}
	case 0x5:
		// print_char
		r, err := zsciiLookup(uint16(ops[0]), true)
		if err != nil {
			return err
		}
		return m.ui.Print(string(r))
	case 0x6:
		// print_num
		return m.ui.Print(fmt.Sprint(int16(ops[0])))
	case 0x8:
		// push
		m.currStackFrame().Push(ops[0])
	case 0x9:
		// pull
		if m.Version() < 6 {
			m.setVariable(uint8(ops[0]), m.currStackFrame().Pop())
		} else {
			return errors.New("multiple stacks not supported yet")
		}
	default:
		return errors.New("VAR opcode not implemented yet")
	}
	return nil
}
