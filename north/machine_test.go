package north

import (
	"reflect"
	"testing"
)

func TestFrameLocals(t *testing.T) {
	f := &stackFrame{Locals: []Word{42, 7, 0xfeed}}
	if val := f.LocalAt(1); val != 42 {
		t.Errorf("f.LocalAt(1) != 42 (got %v)", val)
	}
	if val := f.LocalAt(2); val != 7 {
		t.Errorf("f.LocalAt(2) != 7 (got %v)", val)
	}
	if val := f.LocalAt(3); val != 0xfeed {
		t.Errorf("f.LocalAt(3) != 0xfeed (got %v)", val)
	}
	f.SetLocal(2, 8)
	if val := f.LocalAt(1); val != 42 {
		t.Errorf("After set: f.LocalAt(1) != 42 (got %v)", val)
	}
	if val := f.LocalAt(2); val != 8 {
		t.Errorf("After set: f.LocalAt(2) != 8 (got %v)", val)
	}
	if val := f.LocalAt(3); val != 0xfeed {
		t.Errorf("After set: f.LocalAt(3) != 0xfeed (got %v)", val)
	}
}

func TestFrameStack(t *testing.T) {
	f := new(stackFrame)
	f.Push(4)
	if !reflect.DeepEqual(f.Stack, []Word{4}) {
		t.Errorf("Push 4 != {4} (got %v)", f.Stack)
	}
	f.Push(5)
	if !reflect.DeepEqual(f.Stack, []Word{4, 5}) {
		t.Errorf("Push 5 != {4, 5} (got %v)", f.Stack)
	}
	if w := f.Pop(); w != 5 {
		t.Errorf("Pop != 5 (got %v)", w)
	}
	if !reflect.DeepEqual(f.Stack, []Word{4}) {
		t.Errorf("Pop Stack != {4} (got %v)", f.Stack)
	}
	if w := f.Pop(); w != 4 {
		t.Errorf("Pop != 4 (got %v)", w)
	}
	if len(f.Stack) != 0 {
		t.Errorf("Pop Stack != {} (got %v)", f.Stack)
	}
}

func TestLoadByte(t *testing.T) {
	mem := []byte{5, 0xde, 0xad}
	m := &Machine{memory: mem}
	for i := Address(0); i < Address(len(mem)); i++ {
		if b := m.loadByte(i); b != mem[i] {
			t.Errorf("m.loadByte(%v) != %v (got %v)", i, mem[i], b)
		}
	}
}

func TestLoadWord(t *testing.T) {
	m := &Machine{memory: []byte{0xde, 0xad, 0xbe, 0xef}}
	if w := m.loadWord(0); w != 0xdead {
		t.Errorf("m.loadWord(0) != 0xdead (got %v)", w)
	}
	if w := m.loadWord(2); w != 0xbeef {
		t.Errorf("m.loadWord(2) != 0xbeef (got %v)", w)
	}
}

func TestHeader(t *testing.T) {
	m := &Machine{
		memory: []byte{
			0x03, 0x00, 0x00, 0x58, 0x4e, 0x37, 0x4f, 0x05, 0x3b, 0x21, 0x02, 0xb0, 0x22, 0x71, 0x2e, 0x53,
			0x00, 0x00, 0x38, 0x34, 0x30, 0x37, 0x32, 0x36, 0x01, 0xf0, 0xa5, 0xc6, 0xa1, 0x29, 0x00, 0x00,
		},
	}
	if x := m.Version(); x != 3 {
		t.Errorf("m.Version() != 3 (got %v)", x)
	}
	if x := m.initialPC(); x != 0x4f05 {
		t.Errorf("m.initialPC() != 0x4f05 (got %v)", x)
	}
	if x := m.highMemoryBase(); x != 0x4e37 {
		t.Errorf("m.highMemoryBase() != 0x4e37 (got %v)", x)
	}
	if x := m.dictionaryAddress(); x != 0x3b21 {
		t.Errorf("m.dictionaryAddress() != 0x3b21 (got %v)", x)
	}
	if x := m.objectTableAddress(); x != 0x02b0 {
		t.Errorf("m.objectTableAddress() != 0x02b0 (got %v)", x)
	}
	if x := m.globalVariableTableAddress(); x != 0x2271 {
		t.Errorf("m.globalVariableTableAddress() != 0x2271 (got %v)", x)
	}
	if x := m.staticMemoryBase(); x != 0x2e53 {
		t.Errorf("m.staticMemoryBase() != 0x2e53 (got %v)", x)
	}
	if x := m.abbreviationTableAddress(); x != 0x01f0 {
		t.Errorf("m.abbreviationTableAddress() != 0x01f0 (got %v)", x)
	}
}
