package north

import (
	"bytes"
	"fmt"
	"io"
)

type operandType uint8

const (
	largeConstantOperand operandType = iota
	smallConstantOperand
	variableOperand
	omittedOperand
)

type branchInfo uint16

// Condition returns which boolean value the branch is checking for.
func (b branchInfo) Condition() bool {
	return b&0x8000 != 0
}

// Offset returns the branch offset.  An offset of 0 means return false, an
// offset of 1 means return true, and any other offset means that offset
// minus 2.
func (b branchInfo) Offset() int16 {
	if b&0x4000 != 0 {
		return int16(b >> 8 & 0x3f)
	}
	if b&0x2000 != 0 {
		// negative, sign-extend
		return int16(0xc000 | b)
	}
	return int16(b & 0x3fff)
}

func (b branchInfo) String() string {
	if !b.Condition() {
		return fmt.Sprintf("?~(%+d)", b.Offset())
	}
	return fmt.Sprintf("?(%+d)", b.Offset())
}

type instruction interface {
	Opcode() uint16
	OpcodeNumber() uint8
	NOperand() int
	Operand(i int) (Word, operandType)
	StoreVariable() (uint8, bool)
	BranchInfo() (branchInfo, bool)
}

type longInstruction struct {
	opcode        uint8
	operands      [2]uint8
	storeVariable uint8
	branch        branchInfo
}

func (li longInstruction) Opcode() uint16 {
	return uint16(li.opcode)
}

func (li longInstruction) OpcodeNumber() uint8 {
	return li.opcode & 0x1f
}

func (li longInstruction) NOperand() int {
	return 2
}

func (li longInstruction) Operand(i int) (Word, operandType) {
	var bit uint8
	switch i {
	case 0:
		bit = li.opcode & 0x40
	case 1:
		bit = li.opcode & 0x20
	default:
		return 0, omittedOperand
	}
	if bit == 0 {
		return Word(li.operands[i]), smallConstantOperand
	}
	return Word(li.operands[i]), variableOperand
}

func (li longInstruction) StoreVariable() (uint8, bool) {
	n := li.OpcodeNumber()
	return li.storeVariable, n == 0x08 || n == 0x09 || (n >= 0x0f && n <= 0x19)
}

func (li longInstruction) BranchInfo() (branchInfo, bool) {
	n := li.OpcodeNumber()
	return li.branch, (n >= 0x01 && n <= 0x07) || n == 0x0a
}

func (li *longInstruction) setOperand(i int, val Word) {
	li.operands[i] = uint8(val)
}

func (li *longInstruction) setStoreVariable(v uint8) {
	li.storeVariable = v
}

func (li *longInstruction) setBranch(b branchInfo) {
	li.branch = b
}

type shortInstruction struct {
	opcode        uint8
	operand       Word
	storeVariable uint8
	branch        branchInfo
	text          string
}

func (si shortInstruction) Opcode() uint16 {
	return uint16(si.opcode)
}

func (si shortInstruction) OpcodeNumber() uint8 {
	return si.opcode & 0x0f
}

func (si shortInstruction) NOperand() int {
	if si.opcode&0x30 == 0x30 {
		return 0
	}
	return 1
}

func (si shortInstruction) Operand(i int) (Word, operandType) {
	if i != 0 {
		return 0, omittedOperand
	}
	return Word(si.operand), operandType(si.opcode >> 4 & 0x3)
}

func (si shortInstruction) StoreVariable() (uint8, bool) {
	n := si.OpcodeNumber()
	if si.NOperand() == 0 {
		// TODO: save/restore/catch based on version
		return si.storeVariable, false
	}
	// TODO: 0f is call_1n in version 5
	return si.storeVariable, (n >= 0x01 && n <= 0x04) || n == 0x08 || n == 0x0e || n == 0x0f
}

func (si shortInstruction) BranchInfo() (branchInfo, bool) {
	n := si.OpcodeNumber()
	if si.NOperand() == 0 {
		// TODO: save/restore based on version
		return si.branch, n == 0x0d || n == 0x0f
	}
	return si.branch, n >= 0x00 && n <= 0x02
}

func (si *shortInstruction) setOperand(i int, val Word) {
	si.operand = val
}

func (si *shortInstruction) setStoreVariable(v uint8) {
	si.storeVariable = v
}

func (si *shortInstruction) setBranch(b branchInfo) {
	si.branch = b
}

type variableInstruction struct {
	opcode        uint8
	types         uint16
	operands      [8]Word
	storeVariable uint8
	branch        branchInfo
}

func (vi variableInstruction) Opcode() uint16 {
	return uint16(vi.opcode)
}

func (vi variableInstruction) OpcodeNumber() uint8 {
	return vi.opcode & 0x1f
}

func (vi variableInstruction) is2OP() bool {
	return vi.opcode&0x20 == 0
}

func (vi variableInstruction) NOperand() int {
	for i := uint(0); i < 8; i++ {
		if operandType(vi.types>>(14-i*2)&0x3) == omittedOperand {
			return int(i)
		}
	}
	return 8
}

func (vi variableInstruction) Operand(i int) (Word, operandType) {
	if i < 0 || i >= 8 {
		return 0, omittedOperand
	}
	return vi.operands[i], operandType(vi.types >> (14 - uint(i)*2) & 0x03)
}

func (vi variableInstruction) StoreVariable() (uint8, bool) {
	n := vi.OpcodeNumber()
	if vi.is2OP() {
		_, ok := longInstruction{opcode: n}.StoreVariable()
		return vi.storeVariable, ok
	}
	// TODO: 0x04 version 5, 0x09 version 6
	return vi.storeVariable, n == 0x00 || n == 0x07 || n == 0x0c || (n >= 0x16 && n <= 0x18)
}

func (vi variableInstruction) BranchInfo() (branchInfo, bool) {
	if !vi.is2OP() {
		return vi.branch, false
	}
	_, ok := longInstruction{opcode: vi.OpcodeNumber()}.BranchInfo()
	return vi.branch, ok
}

func (vi *variableInstruction) setOperand(i int, val Word) {
	vi.operands[i] = val
}

func (vi *variableInstruction) setStoreVariable(v uint8) {
	vi.storeVariable = v
}

func (vi *variableInstruction) setBranch(b branchInfo) {
	vi.branch = b
}

type extendedInstruction struct {
	opcode        uint8
	types         uint8
	operands      [4]Word
	storeVariable uint8
	branch        branchInfo
}

func (ei extendedInstruction) Opcode() uint16 {
	return 0xbe00 | uint16(ei.opcode)
}

func (ei extendedInstruction) OpcodeNumber() uint8 {
	return ei.opcode
}

func (ei extendedInstruction) NOperand() int {
	for i := uint(0); i < 4; i++ {
		if operandType(ei.types>>(6-i*2)&0x3) == omittedOperand {
			return int(i)
		}
	}
	return 4
}

func (ei extendedInstruction) Operand(i int) (Word, operandType) {
	if i < 0 || i >= 4 {
		return 0, omittedOperand
	}
	return ei.operands[i], operandType(ei.types >> (6 - uint(i)*2) & 0x03)
}

func (ei extendedInstruction) StoreVariable() (uint8, bool) {
	n := ei.OpcodeNumber()
	return ei.storeVariable, (n >= 0x00 && n <= 0x04) || n == 0x09 || n == 0x0a || n == 0x0c || n == 0x13
}

func (ei extendedInstruction) BranchInfo() (branchInfo, bool) {
	n := ei.OpcodeNumber()
	return ei.branch, n == 0x06 || n == 0x18 || n == 0x1b
}

func (ei *extendedInstruction) setOperand(i int, val Word) {
	ei.operands[i] = val
}

func (ei *extendedInstruction) setStoreVariable(v uint8) {
	ei.storeVariable = v
}

func (ei *extendedInstruction) setBranch(b branchInfo) {
	ei.branch = b
}

func decodeInstruction(r io.Reader, alphaset AlphabetSet, u Unabbreviater) (instruction, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return nil, err
	}

	// Opcode and operand types
	var in interface {
		instruction
		setOperand(i int, val Word)
		setStoreVariable(uint8)
		setBranch(branchInfo)
	}
	switch {
	case buf[0] == 0xbe:
		if _, err := io.ReadFull(r, buf[:2]); err != nil {
			return nil, err
		}
		in = &extendedInstruction{opcode: buf[0], types: buf[1]}
	case buf[0] == 0xec || buf[0] == 0xfa:
		// call_vs2 and call_vn2
		if _, err := io.ReadFull(r, buf[1:3]); err != nil {
			return nil, err
		}
		in = &variableInstruction{opcode: buf[0], types: uint16(buf[1])<<8 | uint16(buf[2])}
	case buf[0]&0xc0 == 0xc0:
		if _, err := io.ReadFull(r, buf[1:2]); err != nil {
			return nil, err
		}
		in = &variableInstruction{opcode: buf[0], types: uint16(buf[1])<<8 | 0xff}
	case buf[0]&0xc0 == 0x80:
		in = &shortInstruction{opcode: buf[0]}
	default:
		in = &longInstruction{opcode: buf[0]}
	}

	// Operands
	for i, n := 0, in.NOperand(); i < n; i++ {
		_, t := in.Operand(i)
		switch t {
		case smallConstantOperand, variableOperand:
			if _, err := io.ReadFull(r, buf[:1]); err != nil {
				return nil, err
			}
			in.setOperand(i, Word(buf[0]))
		case largeConstantOperand:
			if _, err := io.ReadFull(r, buf[:2]); err != nil {
				return nil, err
			}
			in.setOperand(i, Word(buf[0])<<8|Word(buf[1]))
		}
	}

	// Store variable
	if _, ok := in.StoreVariable(); ok {
		if _, err := io.ReadFull(r, buf[:1]); err != nil {
			return nil, err
		}
		in.setStoreVariable(buf[0])
	}

	// Branch info
	if _, ok := in.BranchInfo(); ok {
		if _, err := io.ReadFull(r, buf[:1]); err != nil {
			return nil, err
		}
		if buf[0]&0x40 == 0 {
			if _, err := io.ReadFull(r, buf[1:2]); err != nil {
				return nil, err
			}
			in.setBranch(branchInfo(buf[0])<<8 | branchInfo(buf[1]))
		} else {
			in.setBranch(branchInfo(buf[0]) << 8)
		}
	}

	// Text
	if si, ok := in.(*shortInstruction); ok && (si.opcode == 0xb2 || si.opcode == 0xb3) {
		var err error
		if si.text, err = decodeString(r, alphaset, true, u); err != nil {
			return nil, err
		}
	}

	return in, nil
}

func instructionString(name string, in instruction) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\t", name)
	for i := 0; i < in.NOperand(); i++ {
		if i > 0 {
			fmt.Fprint(&b, " ")
		}
		o, ot := in.Operand(i)
		switch ot {
		case largeConstantOperand, smallConstantOperand:
			fmt.Fprintf(&b, "%v", o)
		case variableOperand:
			fmt.Fprintf(&b, "($%02x)", uint8(o))
		}
	}
	if sv, ok := in.StoreVariable(); ok {
		fmt.Fprintf(&b, " -> ($%02x)", sv)
	}
	if bi, ok := in.BranchInfo(); ok {
		fmt.Fprintf(&b, " %v", bi)
	}
	return b.String()
}

func instructionName2OP(n uint8) string {
	switch n {
	case 0x01:
		return "je"
	case 0x02:
		return "jl"
	case 0x03:
		return "jg"
	case 0x04:
		return "dec_chk"
	case 0x05:
		return "inc_chk"
	case 0x06:
		return "jin"
	case 0x07:
		return "test"
	case 0x08:
		return "or"
	case 0x09:
		return "and"
	case 0x0a:
		return "test_attr"
	case 0x0b:
		return "set_attr"
	case 0x0c:
		return "clear_attr"
	case 0x0d:
		return "store"
	case 0x0e:
		return "insert_obj"
	case 0x0f:
		return "loadw"
	case 0x10:
		return "loadb"
	case 0x11:
		return "get_prop"
	case 0x12:
		return "get_prop_addr"
	case 0x13:
		return "get_next_prop"
	case 0x14:
		return "add"
	case 0x15:
		return "sub"
	case 0x16:
		return "mul"
	case 0x17:
		return "div"
	case 0x18:
		return "mod"
	case 0x19:
		return "call_2s"
	case 0x1a:
		return "call_2n"
	case 0x1b:
		return "set_colour"
	case 0x1c:
		return "throw"
	}
	return ""
}

func (li longInstruction) String() string {
	name := instructionName2OP(li.OpcodeNumber())
	if name == "" {
		name = fmt.Sprintf("2OP:%02x", li.opcode)
	}
	return instructionString(name, li)
}

func (si shortInstruction) String() string {
	var name string
	if si.NOperand() == 0 {
		switch si.OpcodeNumber() {
		case 0x0:
			name = "rtrue"
		case 0x1:
			name = "rfalse"
		case 0x2:
			return instructionString("print", si) + fmt.Sprintf(" %q", si.text)
		case 0x3:
			return instructionString("print_ret", si) + fmt.Sprintf(" %q", si.text)
		case 0x4:
			name = "nop"
		case 0x5:
			name = "save"
		case 0x6:
			name = "restore"
		case 0x7:
			name = "restart"
		case 0x8:
			name = "ret_popped"
		case 0x9:
			// TODO: Catch?
			name = "pop"
		case 0xa:
			name = "quit"
		case 0xb:
			name = "new_line"
		case 0xc:
			name = "show_status"
		case 0xd:
			name = "verify"
		case 0xf:
			name = "piracy"
		default:
			name = fmt.Sprintf("0OP:%02x", si.opcode)
		}
	} else {
		switch si.OpcodeNumber() {
		case 0x0:
			name = "jz"
		case 0x1:
			name = "get_sibling"
		case 0x2:
			name = "get_child"
		case 0x3:
			name = "get_parent"
		case 0x4:
			name = "get_prop_len"
		case 0x5:
			name = "inc"
		case 0x6:
			name = "dec"
		case 0x7:
			name = "print_addr"
		case 0x8:
			name = "call_1s"
		case 0x9:
			name = "remove_obj"
		case 0xa:
			name = "print_obj"
		case 0xb:
			name = "ret"
		case 0xc:
			name = "jump"
		case 0xd:
			name = "print_paddr"
		case 0xe:
			name = "load"
		case 0xf:
			// TODO: call_1n
			name = "not"
		default:
			name = fmt.Sprintf("1OP:%02x", si.opcode)
		}
	}
	return instructionString(name, si)
}

func (vi variableInstruction) String() string {
	var name string
	if vi.is2OP() {
		name = instructionName2OP(uint8(vi.OpcodeNumber()))
		if name == "" {
			name = fmt.Sprintf("VAR:%02x", vi.opcode)
		}
	} else {
		switch vi.OpcodeNumber() {
		case 0x00:
			name = "call_vs"
		case 0x01:
			name = "storew"
		case 0x02:
			name = "storeb"
		case 0x03:
			name = "put_prop"
		case 0x04:
			// TODO: aread
			name = "sread"
		case 0x05:
			name = "print_char"
		case 0x06:
			name = "print_num"
		case 0x07:
			name = "random"
		case 0x08:
			name = "push"
		case 0x09:
			name = "pull"
		case 0x0a:
			name = "split_window"
		case 0x0b:
			name = "set_window"
		case 0x0c:
			name = "call_vs2"
		case 0x0d:
			name = "erase_window"
		case 0x0e:
			name = "erase_line"
		case 0x0f:
			name = "set_cursor"
		case 0x10:
			name = "get_cursor"
		case 0x11:
			name = "set_text_style"
		case 0x12:
			name = "buffer_mode"
		case 0x13:
			name = "output_stream"
		case 0x14:
			name = "input_stream"
		case 0x15:
			name = "sound_effect"
		case 0x16:
			name = "read_char"
		case 0x17:
			name = "scan_table"
		case 0x18:
			name = "not"
		case 0x19:
			name = "call_vn"
		case 0x1a:
			name = "call_vn2"
		case 0x1b:
			name = "tokenise"
		case 0x1c:
			name = "encode_text"
		case 0x1d:
			name = "copy_table"
		case 0x1e:
			name = "print_table"
		case 0x1f:
			name = "check_arg_count"
		default:
			name = fmt.Sprintf("VAR:%02x", vi.opcode)
		}
	}
	return instructionString(name, vi)
}

func (ei extendedInstruction) String() string {
	var name string
	switch ei.OpcodeNumber() {
	case 0x00:
		name = "save"
	case 0x01:
		name = "restore"
	case 0x02:
		name = "log_shift"
	case 0x03:
		name = "art_shift"
	case 0x04:
		name = "set_font"
	case 0x05:
		name = "draw_picture"
	case 0x06:
		name = "picture_data"
	case 0x07:
		name = "erase_picture"
	case 0x08:
		name = "set_margins"
	case 0x09:
		name = "save_undo"
	case 0x0a:
		name = "restore_undo"
	case 0x0b:
		name = "print_unicode"
	case 0x0c:
		name = "check_unicode"
	case 0x10:
		name = "move_window"
	case 0x11:
		name = "window_size"
	case 0x12:
		name = "window_style"
	case 0x13:
		name = "get_wind_prop"
	case 0x14:
		name = "scroll_window"
	case 0x15:
		name = "pop_stack"
	case 0x16:
		name = "read_mouse"
	case 0x17:
		name = "mouse_window"
	case 0x18:
		name = "push_stack"
	case 0x19:
		name = "put_wind_prop"
	case 0x1a:
		name = "print_form"
	case 0x1b:
		name = "make_menu"
	case 0x1c:
		name = "picture_table"
	default:
		name = fmt.Sprintf("EXT:%02x", ei.opcode)
	}
	return instructionString(name, ei)
}
