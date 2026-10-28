package main

import "strings"

// diffLines returns the 1-based numbers of the lines of cur that are not in
// the longest common line sequence with prev: the lines a tutorial step
// changed. Both texts use \n; an empty prev marks every line.
func diffLines(prev, cur string) []int {
	a, b := splitLines(prev), splitLines(cur)
	n, m := len(a), len(b)
	if n == 0 {
		out := make([]int, m)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	// Trim the common head and tail so the table stays small.
	head := 0
	for head < n && head < m && a[head] == b[head] {
		head++
	}
	tail := 0
	for tail < n-head && tail < m-head && a[n-1-tail] == b[m-1-tail] {
		tail++
	}
	x, y := a[head:n-tail], b[head:m-tail]
	// lcs[i][j] is the length of the longest common subsequence of x[i:], y[j:].
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			switch {
			case x[i] == y[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []int
	i, j := 0, 0
	for j < len(y) {
		switch {
		case i < len(x) && x[i] == y[j]:
			i++
			j++
		case i < len(x) && lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			out = append(out, head+j+1)
			j++
		}
	}
	return out
}

// splitLines splits text at \n; a final newline does not start another line.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
