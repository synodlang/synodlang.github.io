package main

import (
	"reflect"
	"testing"
)

func TestDiffLines(t *testing.T) {
	for _, tc := range []struct {
		name, prev, cur string
		want            []int
	}{
		{"empty prev marks all", "", "a\nb\n", []int{1, 2}},
		{"both empty", "", "", nil},
		{"same", "a\nb\n", "a\nb\n", nil},
		{"appended", "a\nb\n", "a\nb\nc\n", []int{3}},
		{"inserted", "a\nc\n", "a\nb\nc\n", []int{2}},
		{"changed", "a\nb\nc\n", "a\nX\nc\n", []int{2}},
		{"deleted only", "a\nb\nc\n", "a\nc\n", nil},
		{"moved line counts as inserted", "a\nb\nc\n", "b\nc\na\n", []int{3}},
		{"duplicate lines", "x\nx\n", "x\nx\nx\n", []int{3}},
		{"blank lines", "a\n\nb\n", "a\n\nnew\n\nb\n", []int{3, 4}},
		{"no final newline", "a\nb", "a\nb\nc", []int{3}},
		{"all different", "a\nb\n", "c\nd\n", []int{1, 2}},
	} {
		got := diffLines(tc.prev, tc.cur)
		if !reflect.DeepEqual(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
			t.Errorf("%s: diffLines(%q, %q) = %v, want %v", tc.name, tc.prev, tc.cur, got, tc.want)
		}
	}
}
