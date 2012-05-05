package north

type dictionary struct {
	Separators []rune
	EntrySize  uint8
	Count      Word
	Base       Address
	Words      map[string]Address
}

func (m *Machine) dictionary() (d dictionary, err error) {
	d.Base = m.dictionaryAddress()
	d.Separators = make([]rune, m.memory[d.Base])
	for i := range d.Separators {
		d.Separators[i], err = zsciiLookup(uint16(m.memory[d.Base+Address(i)+1]), false)
		if err != nil {
			return
		}
	}
	d.Base += 1 + Address(len(d.Separators))

	d.EntrySize = m.memory[d.Base]
	d.Count = m.loadWord(d.Base + 1)
	d.Base += 3
	d.Words = make(map[string]Address, d.Count)

	for i := 0; i < int(d.Count); i++ {
		var s string
		a := d.Base + Address(i)*Address(d.EntrySize)
		s, err = m.loadString(a, false)
		if err != nil {
			return
		}
		d.Words[s] = a
	}
	return
}

type lexWord struct {
	Start int
	End   int
	Word  Address
}

func lex(input []rune, dict dictionary) []lexWord {
	indices := splitWords(input, dict.Separators)
	result := make([]lexWord, len(indices))
	for i := range result {
		result[i].Start = indices[i][0]
		result[i].End = indices[i][1]
		word := string(input[indices[i][0]:indices[i][1]])
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
