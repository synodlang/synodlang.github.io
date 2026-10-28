package main

import (
	"html/template"
	"sort"
	"strconv"
	"strings"
)

// Token is one synod-check/1 token: files[].tokens (line, col, len, role).
// Roles: language, phrase, name, field, type, value, comment. Line and col
// are 1-based and col and len count code points (not bytes or UTF-16 units),
// as editor.js reads them.
type Token struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Len  int    `json:"len"`
	Role string `json:"role"`
}

// moduleTokens runs the native `check --json --tokens --style short` on the
// module in dir (in a temporary copy, so dir is never written) and returns
// the tokens per slash file path (decision 108). dir needs no synod.mod
// (tutorial chapters 1 to 3). Files that fail to lex have partial tokens;
// fragments get none.
func moduleTokens(c *Ctx, dir string) (map[string][]Token, error) {
	var doc struct {
		Format string `json:"format"`
		Files  []struct {
			File   string  `json:"file"`
			Tokens []Token `json:"tokens"`
		} `json:"files"`
	}
	err := WithCopy(dir, func(tmp string) error {
		data, err := c.Synod.CheckTokens(tmp)
		if err != nil {
			return err
		}
		return jsonDoc("synod check --json --tokens", data, &doc)
	})
	if err != nil {
		return nil, err
	}
	if doc.Format != "synod-check/1" {
		return nil, errFormat("synod check --json --tokens", doc.Format)
	}
	out := map[string][]Token{}
	for _, f := range doc.Files {
		out[f.File] = f.Tokens
	}
	return out, nil
}

type formatError struct{ what, got string }

func (e *formatError) Error() string { return e.what + ": unexpected format " + strconv.Quote(e.got) }

func errFormat(what, got string) error { return &formatError{what, got} }

// renderCode returns text as the inside of a <code> element: HTML-escaped,
// with `<span class="t-<role>">` around each token; changed lists lines to
// mark, each wrapped in `<span class="changed">` like editor.js does. With no
// tokens the text is plain. Lines are split at \n, so a final newline leaves
// an empty last line, as in the editor.
func renderCode(text string, toks []Token, changed []int) template.HTML {
	byLine := map[int][]Token{}
	for _, t := range toks {
		byLine[t.Line] = append(byLine[t.Line], t)
	}
	mark := map[int]bool{}
	for _, n := range changed {
		mark[n] = true
	}
	var sb strings.Builder
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		if i > 0 {
			sb.WriteByte('\n')
		}
		if mark[n] {
			sb.WriteString(`<span class="changed">`)
		}
		sb.WriteString(renderLine(line, byLine[n]))
		if mark[n] {
			sb.WriteString(`</span>`)
		}
	}
	return template.HTML(sb.String())
}

// renderLine escapes one line and wraps the runs its tokens cover. Tokens
// address code points; a token that overlaps an earlier one or leaves the
// line is clipped.
func renderLine(line string, toks []Token) string {
	if len(toks) == 0 {
		return template.HTMLEscapeString(line)
	}
	cps := []rune(line)
	roles := make([]string, len(cps))
	sort.SliceStable(toks, func(i, j int) bool { return toks[i].Col < toks[j].Col })
	for _, t := range toks {
		for k := t.Col - 1; k < t.Col-1+t.Len && k < len(cps); k++ {
			if k >= 0 && roles[k] == "" {
				roles[k] = t.Role
			}
		}
	}
	var sb strings.Builder
	for i := 0; i < len(cps); {
		j := i + 1
		for j < len(cps) && roles[j] == roles[i] {
			j++
		}
		esc := template.HTMLEscapeString(string(cps[i:j]))
		if roles[i] == "" {
			sb.WriteString(esc)
		} else {
			sb.WriteString(`<span class="t-` + template.HTMLEscapeString(roles[i]) + `">` + esc + `</span>`)
		}
		i = j
	}
	return sb.String()
}
