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
	Name() string
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

func instructionString(in instruction) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\t", in.Name())
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

func (li longInstruction) String() string {
	return instructionString(li)
}

func (si shortInstruction) String() string {
	s := instructionString(si)
	if si.NOperand() == 0 && (si.OpcodeNumber() == 2 || si.OpcodeNumber() == 3) {
		return s + fmt.Sprintf(" %q", si.text)
	}
	return s
}

func (vi variableInstruction) String() string {
	return instructionString(vi)
}

func (ei extendedInstruction) String() string {
	return instructionString(ei)
}

func (li longInstruction) Name() string {
	switch li.OpcodeNumber() {
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
	return fmt.Sprintf("2OP:%02x", li.OpcodeNumber())
}

func (si shortInstruction) Name() string {
	if si.NOperand() == 0 {
		switch si.OpcodeNumber() {
		case 0x0:
			return "rtrue"
		case 0x1:
			return "rfalse"
		case 0x2:
			return "print"
		case 0x3:
			return "print_ret"
		case 0x4:
			return "nop"
		case 0x5:
			return "save"
		case 0x6:
			return "restore"
		case 0x7:
			return "restart"
		case 0x8:
			return "ret_popped"
		case 0x9:
			// TODO: Catch?
			return "pop"
		case 0xa:
			return "quit"
		case 0xb:
			return "new_line"
		case 0xc:
			return "show_status"
		case 0xd:
			return "verify"
		case 0xf:
			return "piracy"
		}
		return fmt.Sprintf("0OP:%02x", si.opcode)
	}

	switch si.OpcodeNumber() {
	case 0x0:
		return "jz"
	case 0x1:
		return "get_sibling"
	case 0x2:
		return "get_child"
	case 0x3:
		return "get_parent"
	case 0x4:
		return "get_prop_len"
	case 0x5:
		return "inc"
	case 0x6:
		return "dec"
	case 0x7:
		return "print_addr"
	case 0x8:
		return "call_1s"
	case 0x9:
		return "remove_obj"
	case 0xa:
		return "print_obj"
	case 0xb:
		return "ret"
	case 0xc:
		return "jump"
	case 0xd:
		return "print_paddr"
	case 0xe:
		return "load"
	case 0xf:
		// TODO: call_1n
		return "not"
	}
	return fmt.Sprintf("1OP:%02x", si.opcode)
}

func (vi variableInstruction) Name() string {
	if vi.is2OP() {
		return longInstruction{opcode: uint8(vi.OpcodeNumber())}.Name()
	}
	switch vi.OpcodeNumber() {
	case 0x00:
		return "call_vs"
	case 0x01:
		return "storew"
	case 0x02:
		return "storeb"
	case 0x03:
		return "put_prop"
	case 0x04:
		// TODO: sread/aread
		return "read"
	case 0x05:
		return "print_char"
	case 0x06:
		return "print_num"
	case 0x07:
		return "random"
	case 0x08:
		return "push"
	case 0x09:
		return "pull"
	case 0x0a:
		return "split_window"
	case 0x0b:
		return "set_window"
	case 0x0c:
		return "call_vs2"
	case 0x0d:
		return "erase_window"
	case 0x0e:
		return "erase_line"
	case 0x0f:
		return "set_cursor"
	case 0x10:
		return "get_cursor"
	case 0x11:
		return "set_text_style"
	case 0x12:
		return "buffer_mode"
	case 0x13:
		return "output_stream"
	case 0x14:
		return "input_stream"
	case 0x15:
		return "sound_effect"
	case 0x16:
		return "read_char"
	case 0x17:
		return "scan_table"
	case 0x18:
		return "not"
	case 0x19:
		return "call_vn"
	case 0x1a:
		return "call_vn2"
	case 0x1b:
		return "tokenise"
	case 0x1c:
		return "encode_text"
	case 0x1d:
		return "copy_table"
	case 0x1e:
		return "print_table"
	case 0x1f:
		return "check_arg_count"
	}
	return fmt.Sprintf("VAR:%02x", vi.opcode)
}

func (ei extendedInstruction) Name() string {
	switch ei.OpcodeNumber() {
	case 0x00:
		return "save"
	case 0x01:
		return "restore"
	case 0x02:
		return "log_shift"
	case 0x03:
		return "art_shift"
	case 0x04:
		return "set_font"
	case 0x05:
		return "draw_picture"
	case 0x06:
		return "picture_data"
	case 0x07:
		return "erase_picture"
	case 0x08:
		return "set_margins"
	case 0x09:
		return "save_undo"
	case 0x0a:
		return "restore_undo"
	case 0x0b:
		return "print_unicode"
	case 0x0c:
		return "check_unicode"
	case 0x10:
		return "move_window"
	case 0x11:
		return "window_size"
	case 0x12:
		return "window_style"
	case 0x13:
		return "get_wind_prop"
	case 0x14:
		return "scroll_window"
	case 0x15:
		return "pop_stack"
	case 0x16:
		return "read_mouse"
	case 0x17:
		return "mouse_window"
	case 0x18:
		return "push_stack"
	case 0x19:
		return "put_wind_prop"
	case 0x1a:
		return "print_form"
	case 0x1b:
		return "make_menu"
	case 0x1c:
		return "picture_table"
	}
	return fmt.Sprintf("EXT:%02x", ei.opcode)
}
