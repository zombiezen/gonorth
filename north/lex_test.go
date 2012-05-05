package north

import (
	"reflect"
	"testing"
)

func TestSplitWords(t *testing.T) {
	tests := []struct {
		Input   string
		Sep     string
		Indices [][2]int
	}{
		{"", `.,"`, nil},
		{"     \t    ", `.,"`, nil},
		{`.,"`, `.,"`, [][2]int{{0, 1}, {1, 2}, {2, 3}}},
		{",,, ,, ", `.,"`, [][2]int{{0, 1}, {1, 2}, {2, 3}, {4, 5}, {5, 6}}},
		{"a b", `.,"`, [][2]int{{0, 1}, {2, 3}}},
		{"Hello", `.,"`, [][2]int{{0, 5}}},
		{"  Hello", `.,"`, [][2]int{{2, 7}}},
		{"Hello  ", `.,"`, [][2]int{{0, 5}}},
		{"  Hello  ", `.,"`, [][2]int{{2, 7}}},
		{"Hello World", `.,"`, [][2]int{{0, 5}, {6, 11}}},
		{"   Hello World", `.,"`, [][2]int{{3, 8}, {9, 14}}},
		{"Hello World   ", `.,"`, [][2]int{{0, 5}, {6, 11}}},
		{"   Hello World   ", `.,"`, [][2]int{{3, 8}, {9, 14}}},
		{"   Hello   \t  World   ", `.,"`, [][2]int{{3, 8}, {14, 19}}},
		{"fred,go fishing", `.,"`, [][2]int{{0, 4}, {4, 5}, {5, 7}, {8, 15}}},
		{"fred,,go fishing", `.,"`, [][2]int{{0, 4}, {4, 5}, {5, 6}, {6, 8}, {9, 16}}},
		{"fred.,go fishing", `.,"`, [][2]int{{0, 4}, {4, 5}, {5, 6}, {6, 8}, {9, 16}}},
	}
	for _, tt := range tests {
		indices := splitWords([]rune(tt.Input), []rune(tt.Sep))
		if !reflect.DeepEqual(indices, tt.Indices) {
			t.Errorf("splitWords(%q) != %#v (got %#v)", tt.Input, tt.Indices, indices)
		}
	}
}
