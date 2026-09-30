package jinja

import "strings"

type DefKind uint8

const (
	DefMacro DefKind = iota
	DefMaterialization
	DefTest
)

// defKeywords maps the first keyword of a {% ... %} tag to the kind of
// declaration it opens. Anything else (set, if, for, endmacro, ...) is not
// a declaration and is skipped.
var defKeywords = map[string]DefKind{
	"macro":           DefMacro,
	"materialization": DefMaterialization,
	"test":            DefTest,
}

type Param struct {
	Name    string
	Default string // literal source of the default expression, "" if none
}

type Definition struct {
	Kind   DefKind
	Name   string
	Params []Param
	Line   int    // 0-based, of the opening tag
	Doc    string // adjacent {# ... #} comment immediately above, if any
}

// Definitions extracts every macro-like declaration (macro, materialization,
// test) in src, in source order.
func Definitions(src []rune) []Definition {
	tags := ScanTags(src)
	defs := make([]Definition, 0)

	var pendingDoc string
	docEnd := -1

	for _, tag := range tags {
		if tag.Region == RegionComment {
			if doc, ok := parseDocComment(src[tag.Start:tag.End]); ok {
				pendingDoc = doc
				docEnd = tag.End
			} else {
				docEnd = -1
			}
			continue
		}

		def, ok := parseDefinitionTag(src[tag.Start:tag.End], tag.Line)
		if !ok {
			docEnd = -1
			continue
		}

		if docEnd >= 0 && onlyWhitespaceBetween(src, docEnd, tag.Start) {
			def.Doc = pendingDoc
		}
		docEnd = -1

		defs = append(defs, def)
	}

	return defs
}

// parseDefinitionTag parses exactly one {% ... %} tag (including its
// delimiters) and reports the declaration it opens, if any. It returns
// false for tags that aren't macro/materialization/test declarations
// (e.g. {% set %}, {% endmacro %}) or that are too malformed to read a
// name from.
func parseDefinitionTag(tag []rune, line int) (Definition, bool) {
	if len(tag) < 4 || tag[0] != '{' || tag[1] != '%' {
		return Definition{}, false
	}

	i := skipTagOpenControl(tag, 2)
	i = skipSpaces(tag, i)

	kwStart := i
	for i < len(tag) && isIdentRune(tag[i]) {
		i++
	}
	kw := string(tag[kwStart:i])

	kind, ok := defKeywords[kw]
	if !ok {
		return Definition{}, false
	}

	i = skipSpaces(tag, i)

	nameStart := i
	for i < len(tag) && isIdentRune(tag[i]) {
		i++
	}
	name := string(tag[nameStart:i])
	if name == "" {
		return Definition{}, false
	}

	end := tagContentEnd(tag)
	if end < i {
		end = i
	}
	i = skipSpaces(tag, i)

	var params []Param
	switch {
	case i < end && tag[i] == '(':
		closeIdx := matchingDelim(tag, i, end, ')')
		if closeIdx < 0 {
			return Definition{}, false
		}
		params = parseParamList(tag[i+1 : closeIdx])
	case i < end && tag[i] == ',':
		// Parenless declarations, e.g.
		// {% materialization my_materialization, adapter='snowflake' %}
		params = parseParamList(tag[i+1 : end])
	}

	return Definition{
		Kind:   kind,
		Name:   name,
		Params: params,
		Line:   line,
	}, true
}

// skipTagOpenControl advances past the whitespace-control '-' in "{%-",
// starting just past the "{%" itself.
func skipTagOpenControl(tag []rune, i int) int {
	if i < len(tag) && tag[i] == '-' {
		i++
	}
	return i
}

// tagContentEnd returns the offset just past the last meaningful content
// rune in tag, i.e. right before the closing "%}" and any whitespace-control
// '-' before it.
func tagContentEnd(tag []rune) int {
	end := len(tag) - 2 // before "%}"
	if end > 0 && tag[end-1] == '-' {
		end--
	}
	return end
}

func skipSpaces(src []rune, i int) int {
	for i < len(src) && isSpace(src[i]) {
		i++
	}
	return i
}

// matchingDelim returns the index within src of the close rune matching the
// delimiter at openIdx, respecting nested (), [], {} and quoted strings.
// limit is the exclusive upper bound to search within. Returns -1 if the
// closer isn't found before limit.
func matchingDelim(src []rune, openIdx, limit int, close rune) int {
	depth := 0
	var quote rune
	for i := openIdx; i < limit; i++ {
		c := src[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 && c == close {
				return i
			}
		}
	}
	return -1
}

// parseParamList splits raw parameter-list source (already stripped of any
// enclosing parens) into Params, respecting nested brackets and quotes so
// that defaults like `foo(a=[1, 2], b="x, y")` split correctly.
func parseParamList(src []rune) []Param {
	parts := splitTopLevel(src, ',')
	params := make([]Param, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, def := splitFirstTopLevel(part, '=')
		params = append(params, Param{
			Name:    strings.TrimSpace(name),
			Default: strings.TrimSpace(def),
		})
	}
	return params
}

// splitTopLevel splits src on sep, ignoring occurrences nested inside (),
// [], {} or quoted strings.
func splitTopLevel(src []rune, sep rune) []string {
	var parts []string
	var quote rune
	depth := 0
	start := 0

	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		default:
			if c == sep && depth == 0 {
				parts = append(parts, string(src[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, string(src[start:]))
	return parts
}

// splitFirstTopLevel splits s on the first top-level occurrence of sep,
// returning (s, "") if sep never appears at depth 0.
func splitFirstTopLevel(s string, sep rune) (string, string) {
	src := []rune(s)
	var quote rune
	depth := 0

	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		default:
			if c == sep && depth == 0 {
				return string(src[:i]), string(src[i+1:])
			}
		}
	}
	return s, ""
}

// parseDocComment reads a {# ... #} tag (including delimiters) and returns
// its trimmed inner text. It strips whitespace-control markers ({#- and
// -#}) before trimming.
func parseDocComment(tag []rune) (string, bool) {
	if len(tag) < 4 || tag[0] != '{' || tag[1] != '#' {
		return "", false
	}

	i := skipTagOpenControl(tag, 2)
	end := len(tag) - 2 // before "#}"
	if end > i && tag[end-1] == '-' {
		end--
	}
	if end < i {
		end = i
	}

	return strings.TrimSpace(string(tag[i:end])), true
}

// onlyWhitespaceBetween reports whether src[from:to) contains nothing but
// whitespace, i.e. a doc comment ending at from is immediately adjacent to
// a declaration starting at to.
func onlyWhitespaceBetween(src []rune, from, to int) bool {
	for i := from; i < to; i++ {
		if !isSpace(src[i]) {
			return false
		}
	}
	return true
}
