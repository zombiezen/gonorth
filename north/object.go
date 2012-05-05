package north

type object struct {
	Attributes   [6]byte
	Parent       Word
	Sibling      Word
	Child        Word
	PropertyBase Address
}

// Attr returns the value of attribute i.
func (o *object) Attr(i uint8) bool {
	return o.Attributes[i/8]&(1<<(7-i%8)) != 0
}

// SetAttr changes the value of attribute i.
func (o *object) SetAttr(i uint8, val bool) {
	mask := byte(1 << (7 - i%8))
	if val {
		o.Attributes[i/8] |= mask
	} else {
		o.Attributes[i/8] &^= mask
	}
}

// FetchName retrieves the object's name from m's memory.
func (o *object) FetchName(m *Machine) (string, error) {
	// TODO: Is this an output string?
	return m.loadString(o.PropertyBase+1, true)
}

func (o *object) propLoc(m *Machine, i uint8) (Address, uint8) {
	if i == 0 {
		return 0, 0
	}

	a := o.PropertyBase + 1 + Address(m.memory[o.PropertyBase])*2
	if m.Version() <= 3 {
		for m.memory[a] != 0 {
			size, n := m.memory[a]>>5+1, m.memory[a]&0x1f
			a++
			if n == i {
				return a, size
			}
			a += Address(size)
		}
		return 0, 0
	}

	for {
		var size, n uint8
		if m.memory[a]&0x80 == 0 {
			// One-byte
			size, n = m.memory[a]>>5+1, m.memory[a]&0x1f
			a++
		} else {
			// Two-byte
			size, n = m.memory[a+1]&0x1f, m.memory[a]&0x1f
			if n == 0 {
				// Standard 12.4.2.1.1: 0 should be interpreted as 64
				n = 64
			}
			a += 2
		}
		if n == 0 {
			break
		} else if n == i {
			return a, size
		}
		a += Address(size)
	}
	return 0, 0
}

// Property retrieves an object's property i (1-based) from m's memory.  The
// returned slice points to m's memory, or nil if the object doesn't have
// property i.
func (o *object) Property(m *Machine, i uint8) []byte {
	a, size := o.propLoc(m, i)
	if a == 0 {
		return nil
	}
	return m.memory[a : a+Address(size)]
}

// PropertyAddress returns the address of the object's property i (1-based), or
// 0 if not found.
func (o *object) PropertyAddress(m *Machine, i uint8) Address {
	a, _ := o.propLoc(m, i)
	return a
}

// defaultPropertyValue fetches the value that should be returned when querying
// property i on an object that doesn't have property i.
func (m *Machine) defaultPropertyValue(i uint8) Word {
	return m.loadWord(m.objectTableAddress() + Address(i-1)*2)
}

// loadObject returns the record for object i (1-based) in the object table.
func (m *Machine) loadObject(i Word) *object {
	o := new(object)
	if m.Version() <= 3 {
		base := m.objectTableAddress() + (31 * 2) + Address((i-1)*9)
		copy(o.Attributes[:4], m.memory[base:])
		o.Parent = Word(m.memory[base+4])
		o.Sibling = Word(m.memory[base+5])
		o.Child = Word(m.memory[base+6])
		o.PropertyBase = Address(m.loadWord(base + 7))
	} else {
		base := m.objectTableAddress() + (63 * 2) + Address((i-1)*14)
		copy(o.Attributes[:6], m.memory[base:])
		o.Parent = m.loadWord(base + 6)
		o.Sibling = m.loadWord(base + 8)
		o.Child = m.loadWord(base + 10)
		o.PropertyBase = Address(m.loadWord(base + 12))
	}
	return o
}

// storeObject updates the record for object i (1-based) in the object table.
func (m *Machine) storeObject(i Word, o *object) {
	if m.Version() <= 3 {
		base := m.objectTableAddress() + (31 * 2) + Address((i-1)*9)
		copy(m.memory[base:], o.Attributes[:4])
		m.memory[base+4] = byte(o.Parent)
		m.memory[base+5] = byte(o.Sibling)
		m.memory[base+6] = byte(o.Child)
		m.storeWord(base+7, Word(o.PropertyBase))
	} else {
		base := m.objectTableAddress() + (63 * 2) + Address((i-1)*14)
		copy(m.memory[base:], o.Attributes[:6])
		m.storeWord(base+6, o.Parent)
		m.storeWord(base+8, o.Sibling)
		m.storeWord(base+10, o.Child)
		m.storeWord(base+12, Word(o.PropertyBase))
	}
}
