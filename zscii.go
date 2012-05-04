package main

import (
	"errors"
	"fmt"
	"io"
)

// An Unabbreviater fetches ZSCII abbreviations.
type Unabbreviater interface {
	Unabbreviate(entry int) (string, error)
}

var ErrAbbrev = errors.New("Abbreviation not allowed in string")

// A ZSCIIDecodeError is returned when a ZSCII string contains an invalid code point.
type ZSCIIDecodeError struct {
	Code uint16
}

func (e ZSCIIDecodeError) Error() string {
	return fmt.Sprintf("invalid ZSCII code point %#03x", e.Code)
}

type AlphabetSet [3][26]rune

var (
	StandardAlphabetSet = AlphabetSet{
		{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z'},
		{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'},
		{0, '\n', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', ',', '!', '?', '_', '#', '\'', '"', '/', '\\', '-', ':', '(', ')'},
	}
)

func NewZSCIIDecoder(r io.ByteReader, alphaset AlphabetSet, output bool, u Unabbreviater) io.RuneReader {
	d := &zsciiDecoder{r: r, u: u, alphaset: alphaset, output: output}
	d.alphaset[2][0] = 0
	d.alphaset[2][1] = '\n'
	return d
}

type zsciiDecoder struct {
	r        io.ByteReader
	u        Unabbreviater
	abbv []rune
	alphaset AlphabetSet
	output   bool
	err      error
}

func (zd *zsciiDecoder) ReadRune() (r rune, size int, err error) {
	if zd.err != nil {
		return 0, 0, zd.err
	}
	defer func() {
		if err != nil {
			zd.err = err
		}
	}()

	if len(zd.abbv) > 0 {
		r, zd.abbv = zd.abbv[0], zd.abbv[1:]
		return
	}

	alphabet := 0
	z, err := zd.r.ReadByte()
	size++
	if err != nil {
		return
	}

	switch z {
	case 0:
		r = ' '
		return
	case 1, 2, 3:
		var x byte
		if zd.u == nil {
			err = ErrAbbrev
			return
		}
		x, err = zd.r.ReadByte()
		size++
		if err != nil {
			return
		}
		var s string
		s, err = zd.u.Unabbreviate(int(32*(z-1) + x))
		if err != nil {
			return
		}
		zd.abbv = []rune(s)
		r, zd.abbv = zd.abbv[0], zd.abbv[1:]
		return
	case 4, 5:
		for z == 4 || z == 5 {
			alphabet = int(z - 3)
			z, err = zd.r.ReadByte()
			size++
			if err != nil {
				return
			}
		}

		if alphabet == 2 && z == 6 {
			// 10-bit ZSCII character
			var x1, x2 byte
			size++
			if x1, err = zd.r.ReadByte(); err != nil {
				return
			}
			size++
			if x2, err = zd.r.ReadByte(); err != nil {
				return
			}
			r, err = zsciiLookup(uint16(x1)<<5|uint16(x2), zd.output)
			return
		}
	}

	// Alphabet
	r = zd.alphaset[alphabet][z-6]
	return
}

// zsciiLookup returns the rune corresponding to a ZSCII code point.
func zsciiLookup(code uint16, output bool) (r rune, err error) {
	switch {
	case code == 0 && output:
		return 0, nil
	case code == 13:
		return '\n', nil
	case code >= 32 && code <= 126:
		return rune(code), nil
	}
	return 0, ZSCIIDecodeError{code}
}

type zcharReader struct {
	r    io.Reader
	pair [2]byte
	i    int
	err  error
}

func (z *zcharReader) ReadByte() (byte, error) {
	if z.err != nil {
		return 0, z.err
	}

	switch z.i {
	case 0:
		if n, err := io.ReadFull(z.r, z.pair[:]); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			z.err = err
			if n > 0 {
				return z.pair[0] >> 2 & 0x1f, nil
			}
			return 0, z.err
		}
		z.i++
		return z.pair[0] >> 2 & 0x1f, nil
	case 1:
		z.i++
		return z.pair[0]&0x03<<3 | z.pair[1]>>5, nil
	case 2:
		if z.pair[0]&0x80 == 0 {
			z.i = 0
		} else {
			z.i = -1
		}
		return z.pair[1] & 0x1f, nil
	}

	z.err = io.EOF
	return 0, z.err
}

// decodeString decodes a Z-char-encoded ZSCII string from r. alphaset, output,
// and u are the same as in NewZSCIIDecoder.
func decodeString(r io.Reader, alphaset AlphabetSet, output bool, u Unabbreviater) (s string, err error) {
	d := NewZSCIIDecoder(&zcharReader{r: r}, alphaset, output, u)
	ru := make([]rune, 0)
	for {
		var rr rune
		rr, _, err = d.ReadRune()
		if err != nil {
			break
		}
		ru = append(ru, rr)
	}

	s = string(ru)
	if err == io.EOF {
		err = nil
	}
	return
}
