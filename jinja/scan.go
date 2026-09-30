// Package jinja implements a lexical scanner and completion-context
// classifier for Jinja templates as used by dbt. It is deliberately not a
// parser: during completion the buffer is almost always half-typed (an
// unclosed "{{", a dangling "(", ...), and a scanner handles that by
// construction instead of needing error-recovery heuristics.
package jinja

// Region identifies which kind of territory the cursor is in.
type Region uint8

const (
	RegionText    Region = iota // plain SQL, no tag open
	RegionExpr                  // inside {{ ... }}
	RegionStmt                  // inside {% ... %}
	RegionComment               // inside {# ... #}
	RegionRaw                   // inside {% raw %} ... {% endraw %}
)

// frame tracks one level of open parentheses inside a Jinja tag, so we know
// whose argument list the cursor sits in.
type frame struct {
	callee   string   // identifier immediately preceding the '('
	argIndex int      // number of commas seen at this depth
	args     []string // completed string-literal args seen so far, by index
}

// ScanState is the lexical situation at some offset.
type ScanState struct {
	Region Region
	Quote  rune    // the open quote inside a Jinja tag, or 0
	Stack  []frame // innermost call last
	OpenAt int     // offset of the '{{' or '{%' that opened the region
}

// scanner holds the mutable state used while walking src up to cursor.
type scanner struct {
	src        []rune
	pos        int
	region     Region
	quote      rune
	quoteStart int // rune offset of the opening quote of sc.quote, when sc.quote != 0
	stack      []frame
	openAt     int
}

// ScanTo walks src from offset 0 up to cursor and reports the lexical state
// at that point: which kind of region the cursor is in, whether it's inside
// an open string literal, and the stack of open parentheses.
//
// Critically, this scan is blind to SQL quoting and SQL comments outside of
// Jinja tags: dbt renders the Jinja template first and hands the resulting
// text to the database, so a "--" or a "'" in plain SQL text has no bearing
// on whether a Jinja tag is open. Conversely, once inside a Jinja tag,
// quotes are real and do hide the closing delimiter (e.g.
// `{{ f("}}", ...` — the `}}` inside the string does not close the tag).
func ScanTo(src []rune, cursor int) ScanState {
	if cursor > len(src) {
		cursor = len(src)
	}
	if cursor < 0 {
		cursor = 0
	}

	sc := &scanner{src: src}
	for sc.pos < cursor {
		switch sc.region {
		case RegionText:
			sc.scanText(cursor)
		case RegionComment:
			sc.scanComment()
		case RegionRaw:
			sc.scanRaw()
		default: // RegionExpr, RegionStmt
			sc.scanJinja()
		}
	}

	return ScanState{
		Region: sc.region,
		Quote:  sc.quote,
		Stack:  sc.stack,
		OpenAt: sc.openAt,
	}
}

// at reports whether s begins at sc.pos.
func (sc *scanner) at(s string) bool {
	end := sc.pos + len(s)
	if end > len(sc.src) {
		return false
	}
	for i, r := range s {
		if sc.src[sc.pos+i] != r {
			return false
		}
	}
	return true
}

// atFold is like at but case-insensitive, ASCII only (enough for keywords).
func (sc *scanner) atFold(s string) bool {
	end := sc.pos + len(s)
	if end > len(sc.src) {
		return false
	}
	for i, r := range s {
		c := sc.src[sc.pos+i]
		if toLowerRune(c) != toLowerRune(r) {
			return false
		}
	}
	return true
}

func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// scanText handles RegionText. It only looks for tag openers; it must NOT
// treat SQL quotes or SQL comments (`'`, `"`, `--`, `/*`) specially — see the
// package-level doc comment and §2 of the design doc.
func (sc *scanner) scanText(cursor int) {
	if sc.at("{{") {
		sc.openAt = sc.pos
		sc.region = RegionExpr
		sc.pos += 2
		sc.skipWhitespaceControl()
		return
	}
	if sc.at("{#") {
		sc.openAt = sc.pos
		sc.region = RegionComment
		sc.pos += 2
		return
	}
	if sc.at("{%") {
		start := sc.pos
		sc.pos += 2
		sc.skipWhitespaceControl()
		kw := sc.peekKeyword()
		if kw == "raw" {
			sc.region = RegionRaw
		} else {
			sc.region = RegionStmt
		}
		sc.openAt = start
		return
	}
	sc.pos++
}

// skipWhitespaceControl advances past a leading '-' used for whitespace
// control (`{{-`, `{%-`), so the token walk that follows doesn't mistake it
// for an operator.
func (sc *scanner) skipWhitespaceControl() {
	if sc.pos < len(sc.src) && sc.src[sc.pos] == '-' {
		sc.pos++
	}
}

// peekKeyword returns the first identifier-like word after the current
// position, skipping leading whitespace, without consuming it.
func (sc *scanner) peekKeyword() string {
	p := sc.pos
	for p < len(sc.src) && isSpace(sc.src[p]) {
		p++
	}
	start := p
	for p < len(sc.src) && isIdentRune(sc.src[p]) {
		p++
	}
	return string(sc.src[start:p])
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// scanJinja handles RegionExpr and RegionStmt: string-aware token walking
// that tracks open parentheses (to know whose argument list we're in) and
// closes the region on the matching delimiter.
func (sc *scanner) scanJinja() {
	c := sc.src[sc.pos]

	if sc.quote != 0 { // inside a string literal
		switch {
		case c == '\\':
			sc.pos += 2 // skip the escape and whatever it escapes
		case c == sc.quote:
			// Capture the literal text (without quotes) as a completed
			// positional argument of the enclosing call, if any.
			if n := len(sc.stack); n > 0 {
				text := string(sc.src[sc.quoteStart+1 : sc.pos])
				top := &sc.stack[n-1]
				for len(top.args) <= top.argIndex {
					top.args = append(top.args, "")
				}
				top.args[top.argIndex] = text
			}
			sc.quote = 0
			sc.pos++
		default:
			sc.pos++
		}
		return
	}

	switch {
	case c == '\'' || c == '"':
		sc.quote = c
		sc.quoteStart = sc.pos
		sc.pos++
	case sc.region == RegionExpr && sc.at("}}"):
		sc.region, sc.pos = RegionText, sc.pos+2
		sc.stack = sc.stack[:0]
	case sc.region == RegionStmt && sc.at("%}"):
		sc.region, sc.pos = RegionText, sc.pos+2
		sc.stack = sc.stack[:0]
	case c == '(':
		sc.stack = append(sc.stack, frame{callee: sc.identBefore(sc.pos)})
		sc.pos++
	case c == ')':
		if n := len(sc.stack); n > 0 {
			sc.stack = sc.stack[:n-1]
		}
		sc.pos++
	case c == ',':
		if n := len(sc.stack); n > 0 {
			sc.stack[n-1].argIndex++
		}
		sc.pos++
	default:
		sc.pos++
	}
}

// identBefore returns the identifier immediately preceding offset pos
// (skipping no whitespace — callees are directly adjacent to '('), or ""
// if there isn't one.
func (sc *scanner) identBefore(pos int) string {
	end := pos
	start := end
	for start > 0 && isIdentRune(sc.src[start-1]) {
		start--
	}
	if start == end {
		return ""
	}
	return string(sc.src[start:end])
}

// scanComment advances until the closing '#}'.
func (sc *scanner) scanComment() {
	if sc.at("#}") {
		sc.region = RegionText
		sc.pos += 2
		return
	}
	sc.pos++
}

// scanRaw advances until the literal "{% endraw %}" (tolerating whitespace
// control variants like "{%- endraw -%}"). Nothing else exits a raw block.
func (sc *scanner) scanRaw() {
	if sc.at("{%") {
		save := sc.pos
		p := sc.pos + 2
		if p < len(sc.src) && sc.src[p] == '-' {
			p++
		}
		for p < len(sc.src) && isSpace(sc.src[p]) {
			p++
		}
		if sc.atKeywordAt(p, "endraw") {
			p += len("endraw")
			for p < len(sc.src) && isSpace(sc.src[p]) {
				p++
			}
			if p < len(sc.src) && sc.src[p] == '-' {
				p++
			}
			if p+1 < len(sc.src) && sc.src[p] == '%' && sc.src[p+1] == '}' {
				sc.region = RegionText
				sc.pos = p + 2
				return
			}
		}
		sc.pos = save + 1
		return
	}
	sc.pos++
}

// atKeywordAt reports whether the literal word kw begins at offset p.
func (sc *scanner) atKeywordAt(p int, kw string) bool {
	end := p + len(kw)
	if end > len(sc.src) {
		return false
	}
	for i, r := range kw {
		if sc.src[p+i] != r {
			return false
		}
	}
	return true
}

func isIdentRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

// Tag describes one complete {% ... %} or {# ... #} span found while
// walking a whole file, in source order.
type Tag struct {
	Region Region // RegionStmt or RegionComment
	Start  int    // rune offset of the opening delimiter
	End    int    // rune offset just past the closing delimiter
	Line   int    // 0-based line of Start
}

// ScanTags walks all of src exactly once and reports every top-level
// {% ... %} and {# ... #} tag in source order. It reuses the same
// string-aware, raw-block-aware lexing as ScanTo, so tag lookalikes inside
// quotes, comments, or {% raw %} blocks are correctly skipped.
//
// Unlike ScanTo, which reports the lexical state at a single cursor, this
// is for whole-file structural extraction (e.g. jinja.Definitions): it
// needs to know where every tag starts and ends, not just what region one
// offset falls in.
func ScanTags(src []rune) []Tag {
	sc := &scanner{src: src}
	var tags []Tag

	line := 0
	consumed := 0
	advanceLine := func(upTo int) {
		for ; consumed < upTo; consumed++ {
			if src[consumed] == '\n' {
				line++
			}
		}
	}

	for sc.pos < len(src) {
		switch sc.region {
		case RegionText:
			sc.scanText(len(src))
		case RegionComment:
			start := sc.openAt
			for sc.region == RegionComment && sc.pos < len(src) {
				sc.scanComment()
			}
			advanceLine(start)
			tags = append(tags, Tag{Region: RegionComment, Start: start, End: sc.pos, Line: line})
		case RegionRaw:
			for sc.region == RegionRaw && sc.pos < len(src) {
				sc.scanRaw()
			}
		default: // RegionExpr, RegionStmt
			start, region := sc.openAt, sc.region
			for (sc.region == RegionExpr || sc.region == RegionStmt) && sc.pos < len(src) {
				sc.scanJinja()
			}
			if region == RegionStmt {
				advanceLine(start)
				tags = append(tags, Tag{Region: RegionStmt, Start: start, End: sc.pos, Line: line})
			}
		}
	}

	return tags
}
