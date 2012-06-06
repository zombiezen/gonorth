package north

import (
	"errors"
	"fmt"
	"unicode"
)

type instructionError struct {
	PC          Address
	Instruction instruction
	Err         error
}

func (e instructionError) Error() string {
	return fmt.Sprintf("%v @ %v: %v", e.Instruction, e.PC, e.Err)
}

// Step executes the next opcode in the machine.
func (m *Machine) Step() (err error) {
	defer func(pc Address) {
		if err != nil {
			// XXX: What if we messed with the state already (esp. stack)?
			m.currStackFrame().PC = pc
			if ierr, ok := err.(instructionError); ok {
				err = instructionError{pc, ierr.Instruction, ierr.Err}
			}
		}
	}(m.PC())

	r, err := m.MemoryReader(m.PC())
	if err != nil {
		return err
	}
	// TODO: Get story alphabet set
	i, err := decodeInstruction(r, StandardAlphabetSet, m, m.Version())
	if err != nil {
		return instructionError{Err: err}
	}
	//fmt.Printf("\x1b[34m%v\x1b[33m\t%v\x1b[0m\n", m.PC(), i)
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
	case *extendedInstruction:
		return m.stepExtendedInstruction(in)
	}
	return instructionError{Instruction: i, Err: errors.New("Instruction type not implemented yet")}
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
		NArg:          uint8(len(args)),
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
	if address == 0 {
		return nil
	}
	nlocals := int(m.memory[address])
	if nlocals > 15 {
		return errors.New("Routines have a maximum of 15 local variables")
	}
	newFrame := stackFrame{
		PC:     address + 1,
		Locals: make([]Word, nlocals),
		NArg:   uint8(len(args)),
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
		newVal := int16(m.getVariable(uint8(ops[0]))) - 1
		m.setVariable(uint8(ops[0]), Word(newVal))
		return m.conditional(branch, newVal < int16(ops[1]))
	case 0x05:
		// inc_chk
		newVal := int16(m.getVariable(uint8(ops[0]))) + 1
		m.setVariable(uint8(ops[0]), Word(newVal))
		return m.conditional(branch, newVal > int16(ops[1]))
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
	case 0x0c:
		// clear_attr
		obj := m.loadObject(ops[0])
		obj.SetAttr(uint8(ops[1]), false)
		m.storeObject(ops[0], obj)
	case 0x0d:
		// store
		m.setVariable(uint8(ops[0]), ops[1])
	case 0x0e:
		// insert_obj
		m.insertObject(ops[0], ops[1])
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
	case 0x13:
		// get_next_prop
		obj := m.loadObject(ops[0])
		np, err := obj.NextProperty(m, uint8(ops[1]))
		if err != nil {
			return err
		}
		m.setVariable(storeVariable, Word(np))
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
	case 0x19:
		// call_2s
		if ops[0] == 0 {
			return m.routineCall(0, nil, storeVariable)
		} else {
			return m.routineCall(m.packedAddress(ops[0]), ops[1:], storeVariable)
		}
	case 0x1a:
		// call_2n
		if ops[0] == 0 {
			return m.routineNCall(0, nil)
		} else {
			return m.routineNCall(m.packedAddress(ops[0]), ops[1:])
		}

	default:
		return instructionError{Instruction: in, Err: errors.New("2OP opcode not implemented yet")}
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
	case 0x4:
		// get_prop_len
		// Sadly, the operand is the address of the property data, so we have to dupe work to extract the length. Le sigh.
		var size uint8
		if m.Version() <= 3 {
			size = m.memory[Address(ops[0])-1]>>5 + 1
		} else {
			if m.memory[Address(ops[0])-1]&0x80 == 0 {
				// One-byte
				size = m.memory[Address(ops[0])-1]>>6 + 1
			} else {
				// Two-byte
				size = m.memory[Address(ops[0])-1] & 0x3f
				if size == 0 {
					size = 64
				}
			}
		}
		m.setVariable(in.storeVariable, Word(size))
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
		return m.ui.Output(m.window, s)
	case 0x8:
		// call_1s
		if ops[0] == 0 {
			return m.routineCall(0, nil, in.storeVariable)
		} else {
			return m.routineCall(m.packedAddress(ops[0]), nil, in.storeVariable)
		}
	case 0x9:
		// remove_obj
		m.removeObject(ops[0])
	case 0xa:
		// print_obj
		obj := m.loadObject(ops[0])
		// TODO: check obj for nil
		s, err := obj.FetchName(m)
		if err != nil {
			return err
		}
		return m.ui.Output(m.window, s)
	case 0xb:
		// ret
		return m.routineReturn(ops[0])
	case 0xc:
		// jump
		// TODO: do we ever use 0 or 1 offsets here?
		m.currStackFrame().PC += Address(int16(ops[0])) - 2
	case 0xd:
		// print_paddr
		s, err := m.loadString(m.packedAddress(ops[0]), true)
		if err != nil {
			return err
		}
		return m.ui.Output(m.window, s)
	case 0xe:
		// load
		m.setVariable(in.storeVariable, m.getVariable(uint8(ops[0])))
	case 0xf:
		// call_1n
		if ops[0] == 0 {
			return m.routineNCall(0, nil)
		} else {
			return m.routineNCall(m.packedAddress(ops[0]), nil)
		}
	default:
		return instructionError{Instruction: in, Err: errors.New("1OP opcode not implemented yet")}
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
		return m.ui.Output(m.window, in.text)
	case 0x3:
		// print_ret
		if err := m.ui.Output(m.window, in.text + "\n"); err != nil {
			return err
		}
		return m.routineReturn(1)
	case 0x4:
		// nop
	case 0x5:
		// save
		if m.Version() < 4 {
			// TODO: log error?
			err := m.ui.Save(m)
			return m.conditional(in.branch, err == nil)
		} else {
			// TODO
			return errors.New("Version 4 save not implemented")
		}
	case 0x6:
		// restore
		if m.Version() < 4 {
			return m.ui.Restore(m)
		} else {
			// TODO
			return errors.New("Version 4 restore not implemented")
		}
	case 0x7:
		// restart
		return ErrRestart
	case 0x8:
		// ret_popped
		m.routineReturn(m.currStackFrame().Pop())
	case 0x9:
		// pop
		// TODO: v5+ catch
		m.currStackFrame().Pop()
	case 0xa:
		// quit
		return ErrQuit
	case 0xb:
		// new_line
		return m.ui.Output(m.window, "\n")
	case 0xc:
		// show_status
		if m.Version() <= 3 {
			m.refreshStatusLine()
		}
	case 0xd:
		// verify
		// TODO: actually perform verification
		return m.conditional(in.branch, true)
	case 0xf:
		// piracy
		// ARR NO PIRATES HERE
		return m.conditional(in.branch, true)
	default:
		return instructionError{Instruction: in, Err: errors.New("0OP opcode not implemented yet")}
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
		a := Address(ops[0]) + Address(ops[1])
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
		if m.Version() <= 3 {
			m.refreshStatusLine()
		}
		var input []rune
		textAddr := Address(ops[0])
		if m.Version() <= 4 {
			var err error
			input, err = m.ui.Input(int(m.memory[textAddr]) - 1)
			if err != nil {
				return err
			}

			for i := range input {
				// TODO: Ensure input is ZSCII-clean
				input[i] = unicode.ToLower(input[i])
				m.memory[textAddr+1+Address(i)] = byte(input[i])
			}
			m.memory[textAddr+1+Address(len(input))] = 0
		} else {
			var err error
			input, err = m.ui.Input(int(m.memory[Address(ops[0])]))
			if err != nil {
				return err
			}

			base := textAddr + 2
			if n := m.memory[textAddr+1]; n > 0 {
				base += Address(n)
			}
			m.memory[textAddr+1] += byte(len(input))
			for i := range input {
				// TODO: Ensure input is ZSCII-clean
				m.memory[base+Address(i)] = byte(input[i])
				input[i] = unicode.ToLower(input[i])
			}
		}

		if m.Version() < 5 || ops[1] != 0 {
			dict, err := m.dictionary(m.dictionaryAddress())
			if err != nil {
				return err
			}
			m.tokenise(input, dict, Address(ops[1]), true)
		}

		if m.Version() >= 5 {
			// TODO: use actual terminating character
			m.setVariable(in.storeVariable, '\n')
		}
	case 0x5:
		// print_char
		r, err := zsciiLookup(uint16(ops[0]), true)
		if err != nil {
			return err
		}
		return m.ui.Output(m.window, string(r))
	case 0x6:
		// print_num
		return m.ui.Output(m.window, fmt.Sprint(int16(ops[0])))
	case 0x7:
		// random
		if ops[0] == 0 {
			m.seed()
			m.setVariable(in.storeVariable, 0)
		} else {
			m.setVariable(in.storeVariable, m.random(ops[0]))
		}
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
	case 0xa:
		// split_window
		// TODO
	case 0xb:
		// set_window
		m.window = int(ops[0])
	case 0xc:
		// call_vs2
		if ops[0] == 0 {
			return m.routineCall(0, nil, in.storeVariable)
		} else {
			return m.routineCall(m.packedAddress(ops[0]), ops[1:], in.storeVariable)
		}
	case 0xd:
		// erase_window
		// TODO
	case 0xf:
		// set_cursor
		// TODO
	case 0x11:
		// set_text_style
		// TODO
	case 0x12:
		// buffer_mode
		// TODO
	case 0x13:
		// output_stream
		// TODO
	case 0x15:
		// sound_effect
		if player, ok := m.ui.(SoundPlayer); ok {
			if len(ops) == 0 {
				return player.PlaySound(1, -1, 0)
			} else if len(ops) == 1 {
				return player.PlaySound(int(ops[0]), -1, 0)
			}
			switch ops[1] {
			case 1:
				return player.PrepareSound(int(ops[0]))
			case 2:
				// TODO: Version 5+ callback
				if len(ops) < 3 {
					return player.PlaySound(int(ops[0]), -1, 0)
				}
				return player.PlaySound(int(ops[0]), int8(ops[1]&0x00ff), uint8(ops[1]>>8))
			case 3:
				return player.StopSound(int(ops[0]))
			case 4:
				return player.FinishSound(int(ops[0]))
			}
		}
	case 0x16:
		// read_char
		input, _, err := m.ui.ReadRune()
		if err != nil {
			return err
		}
		m.setVariable(in.storeVariable, Word(input))
	case 0x19, 0x1a:
		// call_vn, call_vn2
		if ops[0] == 0 {
			return m.routineNCall(0, nil)
		} else {
			return m.routineNCall(m.packedAddress(ops[0]), ops[1:])
		}
	case 0x1b:
		// tokenise
		var dict dictionary
		var err error
		if len(ops) > 2 && ops[2] != 0 {
			dict, err = m.dictionary(Address(ops[2]))
		} else {
			dict, err = m.dictionary(m.dictionaryAddress())
		}
		if err != nil {
			return err
		}
		textAddr := Address(ops[0])
		n := Address(m.memory[textAddr+1])
		input := make([]rune, n)
		for i := range input {
			input[i] = unicode.ToLower(rune(m.memory[textAddr+2+Address(i)]))
		}
		m.tokenise(input, dict, Address(ops[1]), len(ops) < 3 || ops[3] == 0)
	case 0x1f:
		// check_arg_count
		return m.conditional(in.branch, m.currStackFrame().NArg == uint8(ops[0]))
	default:
		return instructionError{Instruction: in, Err: errors.New("VAR opcode not implemented yet")}
	}
	return nil
}

func (m *Machine) stepExtendedInstruction(in *extendedInstruction) error {
	ops := m.fetchOperands(in)
	switch in.OpcodeNumber() {
	case 0x09:
		// save_undo
		// TODO
	case 0x0b:
		// print_unicode
		m.ui.Output(m.window, string(rune(ops[0])))
	default:
		return instructionError{Instruction: in, Err: errors.New("EXT opcode not implemented yet")}
	}
	_ = ops
	return nil
}
