package jinja

// Role identifies what kind of completion slot the cursor is in.
type Role uint8

const (
	RoleNone        Role = iota // offer nothing
	RoleValue                   // {{ HERE }} | {% set x = HERE %} | {% if HERE %}
	RoleStmtKeyword             // {% HERE %}
	RoleBindingName             // {% set HERE %} | {% macro HERE %} | {% for HERE in %}
	RoleMember                  // {{ adapter.HERE }}
	RoleFilter                  // {{ x | HERE }}
	RoleTest                    // {{ x is HERE }}
	RoleStringArg               // {{ ref('HERE') }}
	RoleKwargName               // {{ star(HERE=... ) }}
)

// Context describes the completion situation at some cursor offset.
type Context struct {
	Region   Region
	Role     Role
	Receiver string // RoleMember: the identifier before the dot
	Callee   string // RoleStringArg / RoleKwargName: enclosing call
	ArgIndex int    // 0-based argument position within Callee
	Prefix   string // text from word start up to the cursor
	Start    int    // rune offset, start of the word under the cursor
	End      int    // rune offset, end of that word — may be past the cursor

	// PreviousArgs holds the completed string-literal arguments of Callee
	// that come before ArgIndex, by position. An entry is "" when that
	// argument wasn't a string literal (e.g. a variable) or wasn't typed.
	// Only populated for RoleStringArg, where knowing an earlier argument
	// (e.g. the source name in source('src', 'tbl|')) is required to
	// resolve the candidate set for the current one.
	PreviousArgs []string
}

// Classify determines what kind of completion is legal at cursor within src.
func Classify(src []rune, cursor int) Context {
	if cursor > len(src) {
		cursor = len(src)
	}
	if cursor < 0 {
		cursor = 0
	}

	st := ScanTo(src, cursor)

	// Dead zones: nothing renders, or no tag is open.
	switch st.Region {
	case RegionText, RegionComment, RegionRaw:
		return Context{Region: st.Region, Role: RoleNone}
	}

	ctx := Context{Region: st.Region}
	ctx.Start, ctx.End, ctx.Prefix = wordAround(src, cursor)

	// Inside a string literal in a Jinja tag: this is an argument value,
	// e.g. ref('ord|'). The candidate set comes from the callee, not macros.
	if st.Quote != 0 {
		ctx.Role = RoleStringArg
		if n := len(st.Stack); n > 0 {
			ctx.Callee = st.Stack[n-1].callee
			ctx.ArgIndex = st.Stack[n-1].argIndex
			ctx.PreviousArgs = append([]string(nil), st.Stack[n-1].args...)
		}
		return ctx
	}

	switch prev := prevSignificant(src, ctx.Start); {
	case prev.is("."):
		ctx.Role = RoleMember
		ctx.Receiver = identBefore(src, prev.start)
	case prev.is("|"):
		ctx.Role = RoleFilter
	case prev.is("is"):
		ctx.Role = RoleTest
	case prev.is("not") && prevIs(src, prev.start, "is"):
		ctx.Role = RoleTest
	case st.Region == RegionStmt && isFirstWordInTag(src, st.OpenAt, ctx.Start):
		ctx.Role = RoleStmtKeyword
	case st.Region == RegionStmt && isBindingSlot(src, st.OpenAt, ctx.Start):
		// {% set NAME %}, {% macro NAME %}, {% for NAME in ... %}
		ctx.Role = RoleBindingName
	case nextSignificantIs(src, ctx.End, "="):
		ctx.Role = RoleKwargName
		if n := len(st.Stack); n > 0 {
			ctx.Callee = st.Stack[n-1].callee
			ctx.ArgIndex = st.Stack[n-1].argIndex
		}
	default:
		ctx.Role = RoleValue
	}
	return ctx
}

// wordAround finds the identifier the cursor sits inside of (or at the edge
// of), walking both directions so mid-word completion works. Prefix is only
// the text before the cursor (what we filter candidates on); [Start, End) is
// the whole word (what gets replaced).
func wordAround(src []rune, cursor int) (start, end int, prefix string) {
	start = cursor
	for start > 0 && isIdentRune(src[start-1]) {
		start--
	}
	end = cursor
	for end < len(src) && isIdentRune(src[end]) {
		end++
	}
	return start, end, string(src[start:cursor])
}

// token is a significant (non-whitespace) piece of source found while
// walking backwards or forwards from some offset.
type token struct {
	text  string
	start int
	end   int
	found bool
}

func (t token) is(s string) bool {
	return t.found && t.text == s
}

// prevSignificant walks backwards from offset, skipping whitespace, and
// returns the token immediately before it: either a single-character
// operator ('.', '|') or an identifier word ("is", "not", ...).
func prevSignificant(src []rune, offset int) token {
	p := offset
	for p > 0 && isSpace(src[p-1]) {
		p--
	}
	if p == 0 {
		return token{}
	}

	c := src[p-1]
	switch c {
	case '.', '|':
		return token{text: string(c), start: p - 1, end: p, found: true}
	}

	if isIdentRune(c) {
		end := p
		start := end
		for start > 0 && isIdentRune(src[start-1]) {
			start--
		}
		return token{text: string(src[start:end]), start: start, end: end, found: true}
	}

	// Some other punctuation (e.g. '(', ',', '='): not one of the tokens we
	// special-case here.
	return token{text: string(c), start: p - 1, end: p, found: true}
}

// prevIs reports whether the significant token before offset equals word.
func prevIs(src []rune, offset int, word string) bool {
	return prevSignificant(src, offset).is(word)
}

// nextSignificantIs reports whether the next non-whitespace rune at or
// after offset is the single character s.
func nextSignificantIs(src []rune, offset int, s string) bool {
	p := offset
	for p < len(src) && isSpace(src[p]) {
		p++
	}
	if p >= len(src) {
		return false
	}
	return string(src[p]) == s
}

// identBefore returns the identifier ending exactly at offset, or "" if
// there isn't one directly adjacent.
func identBefore(src []rune, offset int) string {
	end := offset
	start := end
	for start > 0 && isIdentRune(src[start-1]) {
		start--
	}
	return string(src[start:end])
}

// isFirstWordInTag reports whether the word at [wordStart) is the first
// significant token after the tag opened at openAt — i.e. the keyword
// position, as in `{% |`.
func isFirstWordInTag(src []rune, openAt, wordStart int) bool {
	p := openAt + 2 // past "{%"
	if p < len(src) && src[p] == '-' {
		p++
	}
	for p < len(src) && isSpace(src[p]) {
		p++
	}
	return p == wordStart
}

// tagKeyword returns the first keyword word of the statement tag opened at
// openAt (e.g. "set", "macro", "for", "if", "call", "do").
func tagKeyword(src []rune, openAt int) string {
	p := openAt + 2
	if p < len(src) && src[p] == '-' {
		p++
	}
	for p < len(src) && isSpace(src[p]) {
		p++
	}
	start := p
	for p < len(src) && isIdentRune(src[p]) {
		p++
	}
	return string(src[start:p])
}

// isBindingSlot reports whether wordStart sits in a name-defining position
// within the statement tag opened at openAt:
//   - {% set NAME ... %}   — true up until an '=' has been seen
//   - {% macro NAME ... %} — always true for the name right after 'macro'
//   - {% for NAME in ... %} — true up until 'in' has been seen
func isBindingSlot(src []rune, openAt, wordStart int) bool {
	kw := tagKeyword(src, openAt)

	// Find where the keyword ends, to start scanning from there.
	p := openAt + 2
	if p < len(src) && src[p] == '-' {
		p++
	}
	for p < len(src) && isSpace(src[p]) {
		p++
	}
	p += len(kw)

	switch kw {
	case "set", "macro":
		// Binding as long as we haven't crossed an '=' before wordStart.
		for i := p; i < wordStart; i++ {
			if src[i] == '=' {
				return false
			}
		}
		return true
	case "for":
		// Binding as long as we haven't crossed the loop-variable's
		// trailing " in " before wordStart.
		for i := p; i+1 < wordStart; i++ {
			if isIdentRune(src[i]) {
				continue
			}
		}
		return !hasWordBefore(src, p, wordStart, "in")
	default:
		return false
	}
}

// hasWordBefore reports whether the exact word appears as a standalone
// token anywhere in src[from:to).
func hasWordBefore(src []rune, from, to int, word string) bool {
	wr := []rune(word)
	for i := from; i+len(wr) <= to; i++ {
		if !matchesAt(src, i, wr) {
			continue
		}
		// must be a standalone word: boundaries on both sides.
		if i > 0 && isIdentRune(src[i-1]) {
			continue
		}
		if i+len(wr) < len(src) && isIdentRune(src[i+len(wr)]) {
			continue
		}
		return true
	}
	return false
}

func matchesAt(src []rune, at int, word []rune) bool {
	if at+len(word) > len(src) {
		return false
	}
	for i, r := range word {
		if src[at+i] != r {
			return false
		}
	}
	return true
}
