package main

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file is a strict Markdown subset renderer for the dsl repository's
// documents. Supported blocks: ATX headings, paragraphs, bullet and numbered
// lists (nested, multi-paragraph, tight or loose), pipe tables with alignment,
// fenced code blocks with an info string, block quotes, horizontal rules,
// <details>/<summary> raw HTML and whole-line HTML comments (directives).
// Supported inline markup: code spans, *emphasis*, **strong**, _emphasis_,
// [links](href), ![images](src), <autolinks> and backslash escapes. Anything
// else (setext headings, indented code, reference links, other raw HTML) is an
// error that names the file and line.

// mdKind is the kind of a block.
type mdKind int

const (
	mdkHeading mdKind = iota
	mdkParagraph
	mdkList
	mdkTable
	mdkFence
	mdkQuote
	mdkRule
	mdkHTML
	mdkDirective
)

// mdBlock is one node of the block tree. Which fields are set depends on kind.
type mdBlock struct {
	kind mdKind
	line int // first line, 1-based
	end  int // last line, 1-based

	text string // heading, paragraph and html source; fence body; directive text
	info string // fence info string, trimmed
	// level is the heading level.
	level int

	ordered bool
	start   int
	loose   bool
	items   [][]*mdBlock

	align []string // "", "left", "right" or "center" per column
	head  []string
	rows  [][]string

	children []*mdBlock // block quote content
}

// mdDoc is a parsed document.
type mdDoc struct {
	name   string
	blocks []*mdBlock
}

// mdHeading describes one heading of a document.
type mdHeading struct {
	Level int
	Text  string // plain text, markup removed
	ID    string
	Line  int
}

// mdOptions are the rendering hooks; every hook is optional.
type mdOptions struct {
	// HeadingID chooses the id of a heading from its level and plain text.
	// The default is a mdSlug of the text, made unique within the document.
	HeadingID func(level int, text string) string
	// Link rewrites the destination of a link or autolink.
	Link func(href string) string
	// Code renders a fenced block; false means the default <pre><code>.
	Code func(info, text string) (template.HTML, bool)
	// Directive renders a whole-line HTML comment (the trimmed text between
	// the comment markers); an empty result drops it. Without the hook,
	// comments are dropped.
	Directive func(text string) (template.HTML, error)
	// CodeSpan renders an inline code span; false means the default <code>.
	CodeSpan func(text string) (template.HTML, bool)
}

// mdLine is a source line with its original 1-based number.
type mdLine struct {
	n int
	s string
}

// mdError is a parse or render error at a file and line.
type mdError struct {
	file string
	line int
	msg  string
}

func (e *mdError) Error() string { return fmt.Sprintf("%s:%d: %s", e.file, e.line, e.msg) }

// parseMarkdown parses src, named name in errors.
func parseMarkdown(name string, src []byte) (*mdDoc, error) {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	var lines []mdLine
	for i, s := range strings.Split(text, "\n") {
		lines = append(lines, mdLine{i + 1, mdExpandTabs(s)})
	}
	p := &mdParser{name: name}
	blocks, err := p.blocks(lines)
	if err != nil {
		return nil, err
	}
	return &mdDoc{name: name, blocks: blocks}, nil
}

// mdExpandTabs turns leading tabs into four spaces.
func mdExpandTabs(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if !strings.Contains(s[:i], "\t") {
		return s
	}
	return strings.ReplaceAll(s[:i], "\t", "    ") + s[i:]
}

type mdParser struct {
	name string
}

func (p *mdParser) errf(line int, format string, args ...any) error {
	return &mdError{p.name, line, fmt.Sprintf(format, args...)}
}

func mdIndentOf(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func mdIsBlank(s string) bool { return strings.TrimSpace(s) == "" }

// mdFenceOpen reports the fence character count and info string of an opening
// code fence line.
func mdFenceOpen(s string) (n int, info string, ok bool) {
	if mdIndentOf(s) > 3 {
		return 0, "", false
	}
	t := strings.TrimLeft(s, " ")
	for n < len(t) && t[n] == '`' {
		n++
	}
	if n < 3 {
		return 0, "", false
	}
	info = strings.TrimSpace(t[n:])
	if strings.Contains(info, "`") {
		return 0, "", false
	}
	return n, info, true
}

// mdFenceClose reports whether s closes a fence of n backticks.
func mdFenceClose(s string, n int) bool {
	if mdIndentOf(s) > 3 {
		return false
	}
	t := strings.TrimSpace(s)
	return len(t) >= n && strings.Trim(t, "`") == ""
}

var mdAtxRe = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ ]+(.*?))?(?:[ ]+#+)?[ ]*$`)

func mdAtxHeading(s string) (level int, text string, ok bool) {
	m := mdAtxRe.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	text = m[2]
	if strings.Trim(text, "#") == "" && text != "" {
		text = ""
	}
	return len(m[1]), strings.TrimSpace(text), true
}

// mdIsRule reports whether s is a horizontal rule line.
func mdIsRule(s string) bool {
	if mdIndentOf(s) > 3 {
		return false
	}
	t := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if len(t) < 3 || (t[0] != '-' && t[0] != '*' && t[0] != '_') {
		return false
	}
	return strings.Trim(t, t[:1]) == ""
}

// mdIsSetext reports whether s would make the previous paragraph a setext
// heading.
func mdIsSetext(s string) bool {
	if mdIndentOf(s) > 3 {
		return false
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	return strings.Trim(t, "=") == "" || (len(t) >= 2 && strings.Trim(t, "-") == "")
}

// mdMarker describes a list item marker at the start of a line.
type mdMarker struct {
	indent  int
	ordered bool
	bullet  byte // bullet character or ordered delimiter
	num     int
	width   int // content column relative to the line start
	rest    string
}

func mdListMarker(s string) (mdMarker, bool) {
	var m mdMarker
	m.indent = mdIndentOf(s)
	if m.indent > 3 {
		return m, false
	}
	t := s[m.indent:]
	i := 0
	switch {
	case t != "" && (t[0] == '-' || t[0] == '*' || t[0] == '+'):
		m.bullet = t[0]
		i = 1
	default:
		for i < len(t) && i < 9 && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(t) || (t[i] != '.' && t[i] != ')') {
			return m, false
		}
		m.ordered = true
		m.num, _ = strconv.Atoi(t[:i])
		m.bullet = t[i]
		i++
	}
	if i < len(t) && t[i] != ' ' {
		return m, false
	}
	sp := 0
	for i+sp < len(t) && t[i+sp] == ' ' {
		sp++
	}
	if i+sp == len(t) || sp > 4 {
		// Empty item, or the content is an indented code block: the content
		// starts one column after the marker.
		sp = 1
	}
	m.width = m.indent + i + sp
	if i+sp <= len(t) {
		m.rest = t[i+sp:]
	}
	return m, true
}

func mdIsHTMLStart(s string) bool {
	if mdIndentOf(s) > 3 {
		return false
	}
	t := strings.TrimLeft(s, " ")
	for _, p := range []string{"<details", "</details", "<summary", "</summary"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

func mdIsCommentStart(s string) bool {
	return mdIndentOf(s) <= 3 && strings.HasPrefix(strings.TrimLeft(s, " "), "<!--")
}

// mdInterrupts reports whether s starts a block that can interrupt a paragraph.
func mdInterrupts(s string) bool {
	if _, _, ok := mdFenceOpen(s); ok {
		return true
	}
	if _, _, ok := mdAtxHeading(s); ok {
		return true
	}
	if mdIsRule(s) || mdIsHTMLStart(s) || mdIsCommentStart(s) {
		return true
	}
	if mdIndentOf(s) <= 3 && strings.HasPrefix(strings.TrimLeft(s, " "), ">") {
		return true
	}
	if m, ok := mdListMarker(s); ok && strings.TrimSpace(m.rest) != "" && (!m.ordered || m.num == 1) {
		return true
	}
	return false
}

var mdDelimRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)

func mdIsDelimRow(s string) bool {
	return strings.Contains(s, "-") && (strings.Contains(s, "|") || strings.Contains(s, ":")) && mdDelimRe.MatchString(s)
}

// mdSplitRow splits a table row into trimmed cells; "\|" is a literal pipe.
func mdSplitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '|':
			cur.WriteByte('|')
			i++
		case s[i] == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" || !strings.HasSuffix(strings.TrimSpace(s), "|") {
		cells = append(cells, rest)
	}
	return cells
}

// mdTableStart reports whether lines[i] and lines[i+1] are a table header and
// delimiter row.
func mdTableStart(lines []mdLine, i int) bool {
	return i+1 < len(lines) && strings.Contains(lines[i].s, "|") && mdIsDelimRow(lines[i+1].s) &&
		!mdIsBlank(lines[i].s) && len(mdSplitRow(lines[i].s)) == len(mdSplitRow(lines[i+1].s))
}

func (p *mdParser) blocks(lines []mdLine) ([]*mdBlock, error) {
	var out []*mdBlock
	i := 0
	for i < len(lines) {
		l := lines[i]
		if mdIsBlank(l.s) {
			i++
			continue
		}
		if n, info, ok := mdFenceOpen(l.s); ok {
			b, next, err := p.fence(lines, i, n, info)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
			i = next
			continue
		}
		if level, text, ok := mdAtxHeading(l.s); ok {
			out = append(out, &mdBlock{kind: mdkHeading, line: l.n, end: l.n, level: level, text: text})
			i++
			continue
		}
		if mdIsRule(l.s) {
			out = append(out, &mdBlock{kind: mdkRule, line: l.n, end: l.n})
			i++
			continue
		}
		if mdIsCommentStart(l.s) {
			b, next, err := p.comment(lines, i)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
			i = next
			continue
		}
		if mdIsHTMLStart(l.s) {
			j := i
			var parts []string
			for j < len(lines) && !mdIsBlank(lines[j].s) {
				parts = append(parts, strings.TrimSpace(lines[j].s))
				j++
			}
			out = append(out, &mdBlock{kind: mdkHTML, line: l.n, end: lines[j-1].n, text: strings.Join(parts, "\n")})
			i = j
			continue
		}
		if mdIndentOf(l.s) <= 3 && strings.HasPrefix(strings.TrimLeft(l.s, " "), ">") {
			b, next, err := p.quote(lines, i)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
			i = next
			continue
		}
		if m, ok := mdListMarker(l.s); ok {
			b, next, err := p.list(lines, i, m)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
			i = next
			continue
		}
		if mdIndentOf(l.s) > 3 {
			return nil, p.errf(l.n, "indented code blocks are not supported; use a fence")
		}
		if mdTableStart(lines, i) {
			b, next, err := p.table(lines, i)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
			i = next
			continue
		}
		b, next, err := p.paragraph(lines, i)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
		i = next
	}
	return out, nil
}

func (p *mdParser) fence(lines []mdLine, i, n int, info string) (*mdBlock, int, error) {
	open := lines[i]
	strip := mdIndentOf(open.s)
	var body []string
	for j := i + 1; j < len(lines); j++ {
		if mdFenceClose(lines[j].s, n) {
			text := strings.Join(body, "\n")
			if len(body) > 0 {
				text += "\n"
			}
			return &mdBlock{kind: mdkFence, line: open.n, end: lines[j].n, info: info, text: text}, j + 1, nil
		}
		s := lines[j].s
		k := 0
		for k < strip && k < len(s) && s[k] == ' ' {
			k++
		}
		body = append(body, s[k:])
	}
	return nil, 0, p.errf(open.n, "code fence is not closed")
}

func (p *mdParser) comment(lines []mdLine, i int) (*mdBlock, int, error) {
	first := lines[i]
	var parts []string
	for j := i; j < len(lines); j++ {
		s := lines[j].s
		if j == i {
			s = strings.TrimLeft(s, " ")[len("<!--"):]
		}
		if k := strings.Index(s, "-->"); k >= 0 {
			if strings.TrimSpace(s[k+3:]) != "" {
				return nil, 0, p.errf(lines[j].n, "text after an HTML comment on its line")
			}
			parts = append(parts, s[:k])
			text := strings.TrimSpace(strings.Join(parts, "\n"))
			return &mdBlock{kind: mdkDirective, line: first.n, end: lines[j].n, text: text}, j + 1, nil
		}
		parts = append(parts, s)
	}
	return nil, 0, p.errf(first.n, "HTML comment is not closed")
}

func (p *mdParser) quote(lines []mdLine, i int) (*mdBlock, int, error) {
	var inner []mdLine
	j := i
	for ; j < len(lines); j++ {
		s := lines[j].s
		if mdIndentOf(s) > 3 || !strings.HasPrefix(strings.TrimLeft(s, " "), ">") {
			break
		}
		s = strings.TrimLeft(s, " ")[1:]
		s = strings.TrimPrefix(s, " ")
		inner = append(inner, mdLine{lines[j].n, s})
	}
	kids, err := p.blocks(inner)
	if err != nil {
		return nil, 0, err
	}
	return &mdBlock{kind: mdkQuote, line: lines[i].n, end: lines[j-1].n, children: kids}, j, nil
}

func (p *mdParser) table(lines []mdLine, i int) (*mdBlock, int, error) {
	b := &mdBlock{kind: mdkTable, line: lines[i].n}
	b.head = mdSplitRow(lines[i].s)
	for _, c := range mdSplitRow(lines[i+1].s) {
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right:
			b.align = append(b.align, "center")
		case right:
			b.align = append(b.align, "right")
		case left:
			b.align = append(b.align, "left")
		default:
			b.align = append(b.align, "")
		}
	}
	j := i + 2
	for ; j < len(lines); j++ {
		s := lines[j].s
		if mdIsBlank(s) || !strings.Contains(s, "|") || mdInterrupts(s) {
			break
		}
		cells := mdSplitRow(s)
		if len(cells) > len(b.head) {
			return nil, 0, p.errf(lines[j].n, "table row has %d cells, header has %d", len(cells), len(b.head))
		}
		for len(cells) < len(b.head) {
			cells = append(cells, "")
		}
		b.rows = append(b.rows, cells)
	}
	b.end = lines[j-1].n
	return b, j, nil
}

func (p *mdParser) paragraph(lines []mdLine, i int) (*mdBlock, int, error) {
	var parts []string
	j := i
	for ; j < len(lines); j++ {
		s := lines[j].s
		if mdIsBlank(s) {
			break
		}
		if j > i {
			if mdIsSetext(s) {
				return nil, 0, p.errf(lines[j].n, "setext headings are not supported; use #")
			}
			if mdInterrupts(s) || mdTableStart(lines, j) {
				break
			}
		}
		parts = append(parts, strings.TrimSpace(s))
	}
	return &mdBlock{kind: mdkParagraph, line: lines[i].n, end: lines[j-1].n, text: strings.Join(parts, "\n")}, j, nil
}

func mdSameList(a, b mdMarker) bool { return a.ordered == b.ordered && a.bullet == b.bullet }

func (p *mdParser) list(lines []mdLine, i int, first mdMarker) (*mdBlock, int, error) {
	b := &mdBlock{kind: mdkList, line: lines[i].n, ordered: first.ordered, start: first.num}
	j := i
	for {
		m, _ := mdListMarker(lines[j].s)
		item := []mdLine{{lines[j].n, m.rest}}
		k := j + 1
		inFence, fenceN := false, 0
		if n, _, ok := mdFenceOpen(m.rest); ok {
			inFence, fenceN = true, n
		}
		last := j // index of the last included nonblank line
		for ; k < len(lines); k++ {
			s := lines[k].s
			if mdIsBlank(s) {
				if inFence {
					item = append(item, mdLine{lines[k].n, ""})
					continue
				}
				// Blank lines belong to the item only when its content continues.
				nx := k + 1
				for nx < len(lines) && mdIsBlank(lines[nx].s) {
					nx++
				}
				if nx >= len(lines) || mdIndentOf(lines[nx].s) < m.width {
					break
				}
				for ; k < nx; k++ {
					item = append(item, mdLine{lines[k].n, ""})
				}
				k--
				continue
			}
			var t string
			switch {
			case mdIndentOf(s) >= m.width:
				t = s[m.width:]
			case !inFence && !mdIsBlank(lines[k-1].s) && !mdInterrupts(s) && !mdIsSetext(s):
				t = strings.TrimLeft(s, " ") // lazy continuation
				if _, ok := mdListMarker(s); ok {
					t = ""
				}
			}
			if t == "" {
				break
			}
			if inFence {
				if mdFenceClose(t, fenceN) {
					inFence = false
				}
			} else if n, _, ok := mdFenceOpen(t); ok {
				inFence, fenceN = true, n
			}
			item = append(item, mdLine{lines[k].n, t})
			last = k
		}
		if inFence {
			return nil, 0, p.errf(lines[j].n, "code fence in list item is not closed")
		}
		for len(item) > 0 && item[len(item)-1].s == "" {
			item = item[:len(item)-1]
		}
		kids, err := p.blocks(item)
		if err != nil {
			return nil, 0, err
		}
		for x := 1; x < len(kids); x++ {
			if kids[x].line > kids[x-1].end+1 {
				b.loose = true
			}
		}
		b.items = append(b.items, kids)
		b.end = lines[last].n
		// Is the next nonblank line a sibling item?
		nx := last + 1
		for nx < len(lines) && mdIsBlank(lines[nx].s) {
			nx++
		}
		if nx >= len(lines) {
			return b, nx, nil
		}
		nm, ok := mdListMarker(lines[nx].s)
		if !ok || !mdSameList(first, nm) || mdIsRule(lines[nx].s) {
			return b, last + 1, nil
		}
		if nx > last+1 {
			b.loose = true
		}
		j = nx
	}
}

// headings lists the headings of the document in order, with their ids.
func (d *mdDoc) headings(opts mdOptions) []mdHeading {
	ids := mdNewIDs(opts)
	var out []mdHeading
	mdWalkBlocks(d.blocks, func(b *mdBlock) {
		if b.kind == mdkHeading {
			text := mdPlainInline(b.text)
			out = append(out, mdHeading{b.level, text, ids.next(b.level, text), b.line})
		}
	})
	return out
}

// mdWalkBlocks calls fn for every block in document order, descending into
// lists and block quotes.
func mdWalkBlocks(blocks []*mdBlock, fn func(*mdBlock)) {
	for _, b := range blocks {
		fn(b)
		for _, item := range b.items {
			mdWalkBlocks(item, fn)
		}
		mdWalkBlocks(b.children, fn)
	}
}

// plain returns the text of the document without markup, code blocks and
// directives, one block per line, for search indexing.
func (d *mdDoc) plain() string {
	var sb strings.Builder
	mdWalkBlocks(d.blocks, func(b *mdBlock) {
		switch b.kind {
		case mdkHeading, mdkParagraph:
			sb.WriteString(mdPlainInline(b.text))
			sb.WriteByte('\n')
		case mdkTable:
			for _, row := range append([][]string{b.head}, b.rows...) {
				for k, c := range row {
					if k > 0 {
						sb.WriteByte(' ')
					}
					sb.WriteString(mdPlainInline(c))
				}
				sb.WriteByte('\n')
			}
		}
	})
	return sb.String()
}

// excerpt returns the plain text of the first paragraph, cut at a word
// boundary to at most max runes (with an ellipsis when cut).
func (d *mdDoc) excerpt(max int) string {
	for _, b := range d.blocks {
		if b.kind != mdkParagraph {
			continue
		}
		s := strings.Join(strings.Fields(mdPlainInline(b.text)), " ")
		if utf8.RuneCountInString(s) <= max {
			return s
		}
		r := []rune(s)[:max]
		cut := len(r)
		for cut > 0 && !unicode.IsSpace(r[cut-1]) {
			cut--
		}
		if cut == 0 {
			cut = len(r)
		}
		return strings.TrimRight(string(r[:cut]), " ,;:.") + "…"
	}
	return ""
}

// directives lists the text of every directive comment in document order.
func (d *mdDoc) directives() []string {
	var out []string
	mdWalkBlocks(d.blocks, func(b *mdBlock) {
		if b.kind == mdkDirective {
			out = append(out, b.text)
		}
	})
	return out
}

var mdTagRe = regexp.MustCompile(`<[^>]*>`)

// mdPlainInline renders inline markup to plain text.
func mdPlainInline(src string) string {
	r := &mdRenderer{name: "", opts: mdOptions{}}
	s, err := r.inline(src, 0)
	if err != nil {
		return src
	}
	return html.UnescapeString(mdTagRe.ReplaceAllString(s, ""))
}

var mdNonSlug = regexp.MustCompile(`[^\p{L}\p{N}_ -]+`)

// mdSlug is the default heading id of a text.
func mdSlug(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = mdNonSlug.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, " ", "-")
	if s == "" {
		return "section"
	}
	return s
}

type mdIdAssigner struct {
	opts mdOptions
	seen map[string]int
}

func mdNewIDs(opts mdOptions) *mdIdAssigner {
	return &mdIdAssigner{opts: opts, seen: map[string]int{}}
}

func (a *mdIdAssigner) next(level int, text string) string {
	if a.opts.HeadingID != nil {
		return a.opts.HeadingID(level, text)
	}
	id := mdSlug(text)
	n := a.seen[id]
	a.seen[id] = n + 1
	if n > 0 {
		id += "-" + strconv.Itoa(n)
	}
	return id
}

// render renders the whole document.
func (d *mdDoc) render(opts mdOptions) (template.HTML, error) {
	return d.renderBlocks(d.blocks, opts)
}

// renderBlocks renders a subsequence of the document's top-level blocks, for
// callers that split a document at directives. Heading ids are the same as
// in render, whatever the subsequence.
func (d *mdDoc) renderBlocks(blocks []*mdBlock, opts mdOptions) (template.HTML, error) {
	r := &mdRenderer{name: d.name, opts: opts, ids: map[*mdBlock]string{}}
	ids := mdNewIDs(opts)
	mdWalkBlocks(d.blocks, func(b *mdBlock) {
		if b.kind == mdkHeading {
			r.ids[b] = ids.next(b.level, mdPlainInline(b.text))
		}
	})
	var sb strings.Builder
	if err := r.blocks(&sb, blocks, false); err != nil {
		return "", err
	}
	return template.HTML(sb.String()), nil
}

// mdRenderInline renders inline markup of a text with the options' hooks.
func mdRenderInline(name string, src string, opts mdOptions) (template.HTML, error) {
	r := &mdRenderer{name: name, opts: opts}
	s, err := r.inline(src, 1)
	return template.HTML(s), err
}

type mdRenderer struct {
	name string
	opts mdOptions
	ids  map[*mdBlock]string
}

func (r *mdRenderer) errf(line int, format string, args ...any) error {
	return &mdError{r.name, line, fmt.Sprintf(format, args...)}
}

var mdEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func (r *mdRenderer) blocks(sb *strings.Builder, blocks []*mdBlock, tight bool) error {
	for _, b := range blocks {
		if err := r.block(sb, b, tight); err != nil {
			return err
		}
	}
	return nil
}

func (r *mdRenderer) block(sb *strings.Builder, b *mdBlock, tight bool) error {
	switch b.kind {
	case mdkHeading:
		s, err := r.inline(b.text, b.line)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "<h%d id=\"%s\">%s</h%[1]d>\n", b.level, mdEsc.Replace(r.ids[b]), s)
	case mdkParagraph:
		s, err := r.inline(b.text, b.line)
		if err != nil {
			return err
		}
		if tight {
			sb.WriteString(s)
		} else {
			sb.WriteString("<p>" + s + "</p>\n")
		}
	case mdkRule:
		sb.WriteString("<hr>\n")
	case mdkFence:
		if r.opts.Code != nil {
			if h, ok := r.opts.Code(b.info, b.text); ok {
				sb.WriteString(string(h))
				if !strings.HasSuffix(string(h), "\n") {
					sb.WriteByte('\n')
				}
				return nil
			}
		}
		sb.WriteString("<pre><code")
		if lang := strings.Fields(b.info); len(lang) > 0 {
			sb.WriteString(` class="language-` + mdEsc.Replace(lang[0]) + `"`)
		}
		sb.WriteString(">" + mdEsc.Replace(b.text) + "</code></pre>\n")
	case mdkDirective:
		if r.opts.Directive == nil {
			return nil
		}
		h, err := r.opts.Directive(b.text)
		if err != nil {
			return r.errf(b.line, "directive %q: %v", b.text, err)
		}
		if h != "" {
			sb.WriteString(string(h))
			if !strings.HasSuffix(string(h), "\n") {
				sb.WriteByte('\n')
			}
		}
	case mdkQuote:
		sb.WriteString("<blockquote>\n")
		if err := r.blocks(sb, b.children, false); err != nil {
			return err
		}
		sb.WriteString("</blockquote>\n")
	case mdkHTML:
		return r.rawHTML(sb, b)
	case mdkTable:
		return r.table(sb, b)
	case mdkList:
		return r.list(sb, b)
	}
	return nil
}

var mdHtmlTagRe = regexp.MustCompile(`</?(?:details(?: open)?|summary)>`)

// rawHTML emits the <details> and <summary> tags of an HTML block; the text
// between tags is inline Markdown.
func (r *mdRenderer) rawHTML(sb *strings.Builder, b *mdBlock) error {
	rest := b.text
	for rest != "" {
		loc := mdHtmlTagRe.FindStringSubmatchIndex(rest)
		text := rest
		if loc != nil {
			text = rest[:loc[0]]
		}
		if strings.TrimSpace(text) != "" {
			s, err := r.inline(strings.TrimSpace(text), b.line)
			if err != nil {
				return err
			}
			sb.WriteString(s)
		}
		if loc == nil {
			break
		}
		tag := rest[loc[0]:loc[1]]
		sb.WriteString(tag)
		rest = rest[loc[1]:]
		if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
			sb.WriteByte('\n')
		}
	}
	if !strings.HasSuffix(sb.String(), "\n") {
		sb.WriteByte('\n')
	}
	return nil
}

func (r *mdRenderer) table(sb *strings.Builder, b *mdBlock) error {
	cell := func(tag string, k int, src string) error {
		s, err := r.inline(src, b.line)
		if err != nil {
			return err
		}
		if a := b.align[k]; a != "" {
			fmt.Fprintf(sb, "<%s style=\"text-align:%s\">%s</%[1]s>\n", tag, a, s)
		} else {
			fmt.Fprintf(sb, "<%s>%s</%[1]s>\n", tag, s)
		}
		return nil
	}
	sb.WriteString("<table>\n<thead>\n<tr>\n")
	for k, c := range b.head {
		if err := cell("th", k, c); err != nil {
			return err
		}
	}
	sb.WriteString("</tr>\n</thead>\n")
	if len(b.rows) > 0 {
		sb.WriteString("<tbody>\n")
		for _, row := range b.rows {
			sb.WriteString("<tr>\n")
			for k, c := range row {
				if err := cell("td", k, c); err != nil {
					return err
				}
			}
			sb.WriteString("</tr>\n")
		}
		sb.WriteString("</tbody>\n")
	}
	sb.WriteString("</table>\n")
	return nil
}

func (r *mdRenderer) list(sb *strings.Builder, b *mdBlock) error {
	tag := "ul"
	if b.ordered {
		tag = "ol"
		if b.start != 1 {
			tag = fmt.Sprintf("ol start=\"%d\"", b.start)
		}
	}
	sb.WriteString("<" + tag + ">\n")
	for _, item := range b.items {
		sb.WriteString("<li>")
		for k, kid := range item {
			if !b.loose && kid.kind == mdkParagraph {
				if err := r.block(sb, kid, true); err != nil {
					return err
				}
				continue
			}
			if !b.loose && k > 0 && !strings.HasSuffix(sb.String(), "\n") {
				sb.WriteByte('\n')
			} else if b.loose && k == 0 && kid.kind != mdkParagraph {
				sb.WriteByte('\n')
			}
			if err := r.block(sb, kid, false); err != nil {
				return err
			}
		}
		sb.WriteString("</li>\n")
	}
	sb.WriteString("</" + strings.Fields(tag)[0] + ">\n")
	return nil
}

// mdLineAt returns the source line of byte offset i of src, which starts at base.
func mdLineAt(src string, i, base int) int { return base + strings.Count(src[:i], "\n") }

func mdIsPunct(c byte) bool { return c > ' ' && c < 0x7f && !mdIsAlnum(c) }

func mdIsAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func mdIsSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\t' }

// inline renders inline markup; line is the source line of src's first byte.
func (r *mdRenderer) inline(src string, line int) (string, error) {
	var sb strings.Builder
	i := 0
	for i < len(src) {
		c := src[i]
		switch c {
		case '\\':
			if i+1 < len(src) && src[i+1] == '\n' {
				return "", r.errf(mdLineAt(src, i, line), "hard line breaks are not supported")
			}
			if i+1 < len(src) && mdIsPunct(src[i+1]) {
				sb.WriteString(mdEsc.Replace(src[i+1 : i+2]))
				i += 2
				continue
			}
			sb.WriteByte('\\')
			i++
		case '`':
			n := mdRunLen(src, i, '`')
			if j := mdCloseTicks(src, i+n, n); j >= 0 {
				sb.WriteString(r.codeSpan(src[i+n : j]))
				i = j + n
				continue
			}
			sb.WriteString(src[i : i+n])
			i += n
		case '!', '[':
			start := i
			if c == '!' {
				if i+1 >= len(src) || src[i+1] != '[' {
					sb.WriteByte(c)
					i++
					continue
				}
				i++
			}
			text, dest, title, next, ok := mdLinkAt(src, i)
			if !ok {
				sb.WriteByte(c)
				i = start + 1
				continue
			}
			attrs := ""
			if title != "" {
				attrs = ` title="` + mdEsc.Replace(title) + `"`
			}
			if c == '!' {
				alt := mdPlainInline(text)
				fmt.Fprintf(&sb, `<img src="%s" alt="%s"%s>`, mdEsc.Replace(dest), mdEsc.Replace(alt), attrs)
			} else {
				inner, err := r.inline(text, mdLineAt(src, i, line))
				if err != nil {
					return "", err
				}
				href := dest
				if r.opts.Link != nil {
					href = r.opts.Link(dest)
				}
				fmt.Fprintf(&sb, `<a href="%s"%s>%s</a>`, mdEsc.Replace(href), attrs, inner)
			}
			i = next
		case '<':
			end := strings.IndexByte(src[i:], '>')
			if end > 0 {
				body := src[i+1 : i+end]
				if mdAutolinkRe.MatchString(body) {
					href := body
					if r.opts.Link != nil {
						href = r.opts.Link(body)
					}
					fmt.Fprintf(&sb, `<a href="%s">%s</a>`, mdEsc.Replace(href), mdEsc.Replace(body))
					i += end + 1
					continue
				}
			}
			if mdRawHTMLAt(src[i:]) {
				return "", r.errf(mdLineAt(src, i, line), "raw HTML in text is not supported (write `<...>` in a code span): %q", mdClip(src[i:]))
			}
			sb.WriteString("&lt;")
			i++
		case '*', '_':
			n := mdRunLen(src, i, c)
			if n <= 3 {
				if j := mdEmphasisClose(src, i, n); j >= 0 {
					inner, err := r.inline(src[i+n:j], mdLineAt(src, i, line))
					if err != nil {
						return "", err
					}
					open, cl := "<em>", "</em>"
					switch n {
					case 2:
						open, cl = "<strong>", "</strong>"
					case 3:
						open, cl = "<em><strong>", "</strong></em>"
					}
					sb.WriteString(open + inner + cl)
					i = j + n
					continue
				}
			}
			sb.WriteString(src[i : i+n])
			i += n
		case '&':
			sb.WriteString("&amp;")
			i++
		case '>':
			sb.WriteString("&gt;")
			i++
		case '"':
			sb.WriteString("&quot;")
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String(), nil
}

var mdAutolinkRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]{1,31}:[^\s<>]*$`)

var mdTagStartRe = regexp.MustCompile(`^<(/?)([a-zA-Z][a-zA-Z0-9-]*)`)

// mdHtmlNames are the element names that count as raw HTML in text. Any other
// name in angle brackets (`<version>`, `<pkg>`) is a placeholder and stays
// literal text.
var mdHtmlNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`a abbr b blockquote body br button center code col dd del details dfn div dl dt em font form h1 h2 h3 h4 h5 h6 head hr html i iframe img input ins kbd label li link mark meta nav object ol p pre q s samp script section select small span strike strong style sub summary sup table tbody td textarea tfoot th thead title tr tt u ul var video`) {
		mdHtmlNames[n] = true
	}
}

// mdRawHTMLAt reports whether s, which starts with '<', is a comment, a
// declaration or a tag of a real HTML element.
func mdRawHTMLAt(s string) bool {
	if len(s) > 1 && (s[1] == '!' || s[1] == '?') {
		return true
	}
	m := mdTagStartRe.FindStringSubmatch(s)
	return m != nil && mdHtmlNames[strings.ToLower(m[2])]
}

func mdClip(s string) string {
	if len(s) > 30 {
		return s[:30] + "…"
	}
	return s
}

func (r *mdRenderer) codeSpan(raw string) string {
	s := strings.ReplaceAll(raw, "\n", " ")
	if len(s) > 1 && s[0] == ' ' && s[len(s)-1] == ' ' && strings.Trim(s, " ") != "" {
		s = s[1 : len(s)-1]
	}
	if r.opts.CodeSpan != nil {
		if h, ok := r.opts.CodeSpan(s); ok {
			return string(h)
		}
	}
	return "<code>" + mdEsc.Replace(s) + "</code>"
}

func mdRunLen(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

// mdCloseTicks finds a backtick run of exactly n at or after from.
func mdCloseTicks(s string, from, n int) int {
	for i := from; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		m := mdRunLen(s, i, '`')
		if m == n {
			return i
		}
		i += m
	}
	return -1
}

// mdEmphasisClose returns the index of the closing delimiter run for the
// opening run of n c's at i, or -1.
func mdEmphasisClose(s string, i, n int) int {
	c := s[i]
	if i+n >= len(s) || mdIsSpace(s[i+n]) {
		return -1
	}
	if c == '_' && i > 0 && mdIsAlnum(s[i-1]) {
		return -1
	}
	for j := i + n; j < len(s); {
		switch s[j] {
		case '\\':
			j += 2
		case '`':
			m := mdRunLen(s, j, '`')
			if k := mdCloseTicks(s, j+m, m); k >= 0 {
				j = k + m
			} else {
				j += m
			}
		case c:
			m := mdRunLen(s, j, c)
			if m == n && !mdIsSpace(s[j-1]) && !(c == '_' && j+m < len(s) && mdIsAlnum(s[j+m])) && j > i+n {
				return j
			}
			j += m
		default:
			j++
		}
	}
	return -1
}

// mdLinkAt parses [text](dest "title") with s[i] == '['.
func mdLinkAt(s string, i int) (text, dest, title string, next int, ok bool) {
	depth := 0
	j := i
	for ; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '`':
			m := mdRunLen(s, j, '`')
			if k := mdCloseTicks(s, j+m, m); k >= 0 {
				j = k + m - 1
			} else {
				j += m - 1
			}
		case '[':
			depth++
		case ']':
			depth--
		}
		if depth == 0 {
			break
		}
	}
	if j >= len(s) || s[j] != ']' || j+1 >= len(s) || s[j+1] != '(' {
		return "", "", "", 0, false
	}
	text = s[i+1 : j]
	k := j + 2
	paren := 1
	start := k
	for ; k < len(s); k++ {
		if s[k] == '\\' {
			k++
			continue
		}
		if s[k] == '(' {
			paren++
		} else if s[k] == ')' {
			paren--
			if paren == 0 {
				break
			}
		}
	}
	if k >= len(s) {
		return "", "", "", 0, false
	}
	inner := strings.TrimSpace(s[start:k])
	dest = inner
	if sp := strings.IndexByte(inner, ' '); sp >= 0 {
		dest = inner[:sp]
		t := strings.TrimSpace(inner[sp:])
		if len(t) < 2 || t[0] != '"' || t[len(t)-1] != '"' {
			return "", "", "", 0, false
		}
		title = t[1 : len(t)-1]
	}
	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
	if dest == "" {
		return "", "", "", 0, false
	}
	return text, dest, title, k + 1, true
}
