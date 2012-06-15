package north

type dictionary struct {
	Separators []rune
	EntrySize  uint8
	Count      Word
	Base       Address
	Words      map[string]Address
	WordSize   int
}

func (m *Machine) dictionary(addr Address) (*dictionary, error) {
	d := &dictionary{
		Base:       addr,
		Separators: make([]rune, m.memory[addr]),
	}
	for i := range d.Separators {
		var err error
		d.Separators[i], err = zsciiLookup(uint16(m.memory[d.Base+Address(i)+1]), false)
		if err != nil {
			return nil, err
		}
	}
	d.Base += 1 + Address(len(d.Separators))

	d.EntrySize = m.memory[d.Base]
	d.Count = m.loadWord(d.Base + 1)
	if i := int16(d.Count); i < 0 {
		// XXX: This may not be right for the game dictionary.
		d.Count = Word(-i)
	}
	d.Base += 3
	d.Words = make(map[string]Address, d.Count)
	if m.Version() <= 3 {
		d.WordSize = 6
	} else {
		d.WordSize = 9
	}

	for i := 0; i < int(d.Count); i++ {
		a := d.Base + Address(i)*Address(d.EntrySize)
		s, err := m.loadString(a, false)
		if err != nil {
			return nil, err
		}
		d.Words[s] = a
	}
	return d, nil
}

// tokenise performs lexical analysis on input using dict, storing the result at
// addr. If storeZero is false, then the parse info for any unrecognized words
// is left unchanged.
func (m *Machine) tokenise(input []rune, dict *dictionary, addr Address, storeZero bool) {
	words := lex(input, dict)
	maxWords := int(m.memory[addr])
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	m.memory[addr+1] = byte(len(words))
	base := addr + 2
	version := m.Version()
	for i := range words {
		if storeZero || words[i].Word != 0 {
			m.storeWord(base+Address(i)*4, Word(words[i].Word))
			m.memory[base+Address(i)*4+2] = byte(words[i].End - words[i].Start)
			if version <= 4 {
				m.memory[base+Address(i)*4+3] = byte(words[i].Start + 1)
			} else {
				m.memory[base+Address(i)*4+3] = byte(words[i].Start + 2)
			}
		}
	}
}

type lexWord struct {
	Start int
	End   int
	Word  Address
}

func lex(input []rune, dict *dictionary) []lexWord {
	indices := splitWords(input, dict.Separators)
	result := make([]lexWord, len(indices))
	for i := range result {
		result[i].Start = indices[i][0]
		result[i].End = indices[i][1]
		word := string(input[indices[i][0]:indices[i][1]])
		if len(word) > dict.WordSize {
			word = word[:dict.WordSize]
		}
		result[i].Word = dict.Words[word]
	}
	return result
}

func splitWords(s, sep []rune) (indices [][2]int) {
	start := -1
	inWord := false
	for i := range s {
		if s[i] == ' ' || s[i] == '\t' {
			if inWord {
				indices = append(indices, [2]int{start, i})
				inWord = false
			}
			continue
		}

		var isSep bool
		for _, r := range sep {
			if s[i] == r {
				isSep = true
				break
			}
		}

		if isSep {
			if inWord {
				indices = append(indices, [2]int{start, i})
				inWord = false
			}
			indices = append(indices, [2]int{i, i + 1})
		} else if !inWord {
			start = i
			inWord = true
		}
	}
	if inWord {
		indices = append(indices, [2]int{start, len(s)})
	}
	return
}
