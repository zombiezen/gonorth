package main

import (
	"errors"
	"fmt"
)

// Step executes the next opcode in the machine.
func (m *Machine) Step() error {
	r, err := m.memoryReader(m.pc)
	if err != nil {
		return err
	}
	// TODO: Get story alphabet set
	i, err := decodeInstruction(r, StandardAlphabetSet, m)
	if err != nil {
		return err
	}
	fmt.Printf("\x1b[34m%v\x1b[33m\t%v\x1b[0m\n", m.pc, i)
	newPC, _ := r.Seek(0, 1)
	m.pc = Address(newPC)

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
		Return:        m.pc,
		Locals:        make([]Word, nlocals),
		Store:         true,
		StoreVariable: ret,
	}
	if m.Version() <= 4 {
		for i := 0; i < nlocals; i++ {
			// XXX: Should this be in reverse order?
			newFrame.Locals[i] = m.loadWord(address + 1 + Address(i)*2)
		}
		m.pc = address + 1 + Address(nlocals)*2
	} else {
		m.pc = address + 1
	}
	for i := range args {
		newFrame.Set(i+1, args[i])
	}
	m.stack = append(m.stack, newFrame)
	//fmt.Printf(">> ROUTINE %04x -> (%#02x): %d locals\n", m.pc, newFrame.StoreVariable, nlocals)
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
	m.pc = frame.Return
	m.stack = m.stack[:len(m.stack)-1]
	if frame.Store {
		m.setVariable(frame.StoreVariable, val)
	}
	//fmt.Printf("<< RETURN %v PC:%04x\n", val, m.pc)
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
			// Purposefully allowing unsigned overflow.
			m.pc += Address(branch.Offset() - 2)
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
		return m.conditional(branch, int16(ops[0]) == int16(ops[1]))
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
	case 0x08:
		// or
		m.setVariable(storeVariable, ops[0]|ops[1])
	case 0x09:
		// and
		m.setVariable(storeVariable, ops[0]&ops[1])
	case 0x0a:
		// test_attr
		obj := m.fetchObject(ops[0])
		return m.conditional(branch, obj.Attr(uint8(ops[1])))
	case 0x0d:
		// store
		m.setVariable(uint8(ops[0]), ops[1])
	case 0x0f:
		// loadw
		m.setVariable(storeVariable, m.loadWord(Address(ops[0]+2*ops[1])))
	case 0x10:
		// loadb
		// TODO: should this be sign extended?
		m.setVariable(storeVariable, Word(m.memory[ops[0]+ops[1]]))
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
	case 0xb:
		// ret
		return m.routineReturn(ops[0])
	case 0xc:
		// jump
		// TODO: do we ever use 0 or 1 offsets here?
		m.pc += Address(ops[0] - 2)
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
		m.storeWord(Address(ops[0])+Address(2*ops[1]), ops[2])
	case 0x2:
		// storeb
		m.memory[Address(ops[0])+Address(2*ops[1])] = byte(ops[2])
	case 0x3:
		// put_prop
		obj := m.fetchObject(ops[0])
		p := obj.Property(m, uint8(ops[1]))
		switch len(p) {
		case 1:
			p[0] = byte(ops[2]&0xff)
		case 2:
			p[0] = byte(ops[2]>>8)
			p[1] = byte(ops[2]&0xff)
		default:
			return fmt.Errorf("Mismatched property size: vs. %d", len(p))
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
	case 0x9:
		// pull
		if m.Version() < 6 {
			m.setVariable(uint8(ops[0]), m.currStackFrame().PopLocal())
		} else {
			return errors.New("multiple stacks not supported yet")
		}
	default:
		return errors.New("VAR opcode not implemented yet")
	}
	return nil
}
