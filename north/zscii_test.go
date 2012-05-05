package north

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestZCharReader(t *testing.T) {
	tests := []struct {
		Input  []byte
		Output []byte
		Err    error
	}{
		{nil, nil, io.ErrUnexpectedEOF},
		{[]byte{0x92}, []byte{0x04}, io.ErrUnexpectedEOF},
		{[]byte{0x94, 0xa5}, []byte{0x05, 0x05, 0x05}, io.EOF},
		{[]byte{0x86, 0x49}, []byte{0x01, 0x12, 0x09}, io.EOF},
		{[]byte{0x06, 0x49}, []byte{0x01, 0x12, 0x09}, io.ErrUnexpectedEOF},
	}

	for i := range tests {
		b := bytes.NewBuffer(tests[i].Input)
		r := zcharReader{r: b}
		result := make([]byte, 0, len(tests[i].Output))
		for {
			c, err := r.ReadByte()
			if err == nil {
				result = append(result, c)
			} else {
				if c != 0 {
					t.Errorf("tests[%d] error byte != 0 (got %q)", i, c)
				}
				if err != tests[i].Err {
					t.Errorf("tests[%d] unexpected error: %v", i, err)
				}
				break
			}
		}
		if !bytes.Equal(tests[i].Output, result) {
			t.Errorf("tests[%d] != %v (got %v)", i, tests[i].Output, result)
		}
	}
}

type mockUnabbreviater struct{}

func (u mockUnabbreviater) Unabbreviate(entry int) (string, error) {
	return fmt.Sprintf("entry%d", entry), nil
}

func TestZSCIIDecoder(t *testing.T) {
	tests := []struct {
		IsOutput      bool
		Unabbreviater Unabbreviater
		Input         []byte
		String        string
		Err           error
	}{
		{true, nil, nil, "", io.EOF},
		{true, nil, []byte{0x4, 0x0, 0x4}, " ", io.EOF},
		{true, nil, []byte{0x4, 0x4, 0x4}, "", io.EOF},
		{true, nil, []byte{0x5, 0x5, 0x5}, "", io.EOF},
		{true, nil, []byte{0x4, 0xd, 0xa, 0x11, 0x11, 0x14, 0x5, 0x13, 0x0, 0x4, 0x1c, 0x14, 0x17, 0x11, 0x9, 0x5, 0x14}, "Hello, World!", io.EOF},
		{true, nil, []byte{0x4, 0xd, 0xa, 0x11, 0x11, 0x14, 0x5, 0x13, 0x0, 0x4, 0x1c, 0x14, 0x17, 0x11, 0x9, 0x5, 0x14, 0x5}, "Hello, World!", io.EOF},
		{true, nil, []byte{0x6, 0x5, 0x6, 0x0, 0xd}, "a\n", io.EOF},
		{true, nil, []byte{0x6, 0x5, 0x6, 0x0}, "a", io.EOF},
		{true, nil, []byte{0x1, 0x4}, "", ErrAbbrev},
		{true, mockUnabbreviater{}, []byte{0x1, 0x4}, "entry4", io.EOF},
		{true, mockUnabbreviater{}, []byte{0x2, 0x4}, "entry36", io.EOF},
		{true, mockUnabbreviater{}, []byte{0x1, 0x0}, "entry0", io.EOF},
	}

	for i := range tests {
		b := bytes.NewBuffer(tests[i].Input)
		d := NewZSCIIDecoder(b, StandardAlphabetSet, tests[i].IsOutput, tests[i].Unabbreviater)
		result := make([]rune, 0, len(tests[i].String))
		for {
			r, _, err := d.ReadRune()
			if err == nil {
				result = append(result, r)
			} else {
				if r != 0 {
					t.Errorf("tests[%d] error rune != 0 (got %q)", i, r)
				}
				if err != tests[i].Err {
					t.Errorf("tests[%d] unexpected error: %v", i, err)
				}
				break
			}
		}

		if string(result) != tests[i].String {
			t.Errorf("tests[%d] != %q (got %q)", i, tests[i].String, string(result))
		}
	}
}
