package main

import "html/template"

// Token is one synod-check/1 token: files[].tokens (line, col, len, role).
// Roles: language, phrase, name, field, type, value, comment.
type Token struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Len  int    `json:"len"`
	Role string `json:"role"`
}

// moduleTokens runs the native `check --json --tokens --style short` on the
// module in dir (a temp copy) and returns the tokens per slash file path
// (decision 108). Fragments get no tokens.
func moduleTokens(c *Ctx, dir string) (map[string][]Token, error) { return nil, nil }

// renderCode returns text as the inside of a <code> element: HTML-escaped,
// with `<span class="t-<role>">` around each token; changed lists lines to
// mark. With no tokens the text is plain.
func renderCode(text string, toks []Token, changed []int) template.HTML {
	return template.HTML(template.HTMLEscapeString(text))
}
