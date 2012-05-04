package main

type object struct {
	Attributes      [6]byte
	Parent          Word
	Sibling         Word
	Child           Word
	PropertyAddress Address
}

// Attr returns the value of attribute i.
func (o *object) Attr(i uint8) bool {
	return o.Attributes[i/8]&(1<<(7-i%8)) != 0
}

// FetchName retrieves the object's name from m's memory.
func (o *object) FetchName(m *Machine) (string, error) {
	// TODO: Is this an output string?
	return m.loadString(o.PropertyAddress+1, true)
}

// Property retrieves an object's property i (1-based) from m's memory.  The
// returned slice points to m's memory, or nil if the object doesn't have
// property i.
func (o *object) Property(m *Machine, i uint8) []byte {
	if i == 0 {
		return nil
	}

	a := o.PropertyAddress + Address(m.memory[o.PropertyAddress])*2 + 1
	if m.Version() <= 3 {
		for m.memory[a] != 0 {
			size, n := m.memory[a]>>5+1, m.memory[a]&0x1f
			if n == i {
				return m.memory[a : a+Address(size)]
			}
			a += 1 + Address(size)
		}
		return nil
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
			return m.memory[a : a+Address(size)]
		}
		a += Address(size)
	}
	return nil
}

// defaultPropertyValue fetches the value that should be returned when querying
// property i on an object that doesn't have property i.
func (m *Machine) defaultPropertyValue(i uint8) Word {
	return m.loadWord(m.objectTableAddress() + Address(i) * 2)
}

// fetchObject returns the record for object i (1-based) in the object table.
func (m *Machine) fetchObject(i Word) *object {
	o := new(object)
	if m.Version() <= 3 {
		base := m.objectTableAddress() + (31 * 2) + Address((i-1)*9)
		copy(o.Attributes[:4], m.memory[base:base+4])
		o.Parent = Word(m.memory[base+4])
		o.Sibling = Word(m.memory[base+5])
		o.Child = Word(m.memory[base+6])
		o.PropertyAddress = Address(m.loadWord(base + 7))
	} else {
		base := m.objectTableAddress() + (63 * 2) + Address((i-1)*14)
		copy(o.Attributes[:4], m.memory[base:base+6])
		o.Parent = m.loadWord(base + 6)
		o.Sibling = m.loadWord(base + 8)
		o.Child = m.loadWord(base + 10)
		o.PropertyAddress = Address(m.loadWord(base + 12))
	}
	return o
}
