package north

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDecodeInstruction(t *testing.T) {
	tests := []struct {
		Input    []byte
		Expected instruction
	}{
		{
			[]byte{0x0b, 0x02, 0x03},
			&longInstruction{opcode: 0x0b, operands: [2]uint8{2, 3}},
		},
		{
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05},
			&longInstruction{opcode: 1, operands: [2]uint8{2, 3}, branch: branchInfo(0x0405)},
		},
		{
			[]byte{0x01, 0x02, 0x03, 0x44},
			&longInstruction{opcode: 1, operands: [2]uint8{2, 3}, branch: branchInfo(0x4400)},
		},
		{
			[]byte{0x85, 0xde, 0xad},
			&shortInstruction{opcode: 0x85, operand: 0xdead},
		},
		{
			[]byte{0x95, 0x42},
			&shortInstruction{opcode: 0x95, operand: 0x42},
		},
		{
			[]byte{0xb0},
			&shortInstruction{opcode: 0xb0},
		},
		{
			[]byte{0xb2, 0x91, 0xae},
			&shortInstruction{opcode: 0xb2, text: "Hi"},
		},
		{
			[]byte{0xc1, 0x8f, 0x00, 0x07, 0xff, 0x04, 0x05},
			&variableInstruction{opcode: 0xc1, types: 0x8fff, operands: [8]Word{0x0000, 0x07ff}, branch: 0x0405},
		},
		{
			[]byte{0xc9, 0x8f, 0x00, 0x07, 0xff, 0x01},
			&variableInstruction{opcode: 0xc9, types: 0x8fff, operands: [8]Word{0x0000, 0x07ff}, storeVariable: 0x01},
		},
		{
			[]byte{0xfa, 0xff, 0xff},
			&variableInstruction{opcode: 0xfa, types: 0xffff},
		},
		{
			[]byte{0xfa, 0x00, 0x00, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xfa, 0x11, 0xfe, 0xe1, 0xde, 0xad, 0xca, 0xfe, 0xba, 0xbe},
			&variableInstruction{opcode: 0xfa, types: 0x0000, operands: [8]Word{0xdead, 0xbeef, 0xdead, 0xfa11, 0xfee1, 0xdead, 0xcafe, 0xbabe}},
		},
		{
			[]byte{0xfa, 0x00, 0x0f, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xfa, 0x11, 0xfe, 0xe1, 0xde, 0xad},
			&variableInstruction{opcode: 0xfa, types: 0x000f, operands: [8]Word{0xdead, 0xbeef, 0xdead, 0xfa11, 0xfee1, 0xdead}},
		},
		{
			[]byte{0xfa, 0x00, 0x07, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xfa, 0x11, 0xfe, 0xe1, 0xde, 0xad, 0x42},
			&variableInstruction{opcode: 0xfa, types: 0x0007, operands: [8]Word{0xdead, 0xbeef, 0xdead, 0xfa11, 0xfee1, 0xdead, 0x42}},
		},
		{
			[]byte{0xbe, 0x05, 0xff},
			&extendedInstruction{opcode: 0x05, types: 0xff},
		},
		{
			[]byte{0xbe, 0x05, 0x57, 0x01, 0x02, 0x03},
			&extendedInstruction{opcode: 0x05, types: 0x57, operands: [4]Word{1, 2, 3}},
		},
	}

	for i, tt := range tests {
		b := bytes.NewBuffer(tt.Input)
		if result, err := decodeInstruction(b, StandardAlphabetSet, nil); err != nil {
			t.Errorf("[%d] error: %v", i, err)
		} else if !reflect.DeepEqual(result, tt.Expected) {
			t.Errorf("[%d] != %#v (got %#v)", i, tt.Expected, result)
		}
	}
}

func TestBranchInfo(t *testing.T) {
	tests := []struct {
		Input     branchInfo
		Condition bool
		Offset    int16
	}{
		{branchInfo(0x7f00), false, 63},
		{branchInfo(0xff00), true, 63},
		{branchInfo(0x4000), false, 0},
		{branchInfo(0xc000), true, 0},
		{branchInfo(0x4100), false, 1},
		{branchInfo(0xc100), true, 1},
		{branchInfo(0x2000), false, -(1 << 13)},
		{branchInfo(0xa000), true, -(1 << 13)},
		{branchInfo(0x3fff), false, -1},
		{branchInfo(0xbfff), true, -1},
		{branchInfo(0x0000), false, 0},
		{branchInfo(0x8000), true, 0},
		{branchInfo(0x0001), false, 1},
		{branchInfo(0x8001), true, 1},
		{branchInfo(0x1fff), false, 1<<13 - 1},
		{branchInfo(0x9fff), true, 1<<13 - 1},
	}

	for i, tt := range tests {
		if tt.Input.Condition() != tt.Condition {
			t.Errorf("[%d] branchInfo(%#04x).Condition() != %v (got %v)", i, uint16(tt.Input), tt.Condition, tt.Input.Condition())
		}
		if tt.Input.Offset() != tt.Offset {
			t.Errorf("[%d] branchInfo(%#04x).Offset() != %d (got %d)", i, uint16(tt.Input), tt.Offset, tt.Input.Offset())
		}
	}
}
