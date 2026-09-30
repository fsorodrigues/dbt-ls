package jinja

import (
	"strings"
	"testing"
)

// caret is the cursor marker used in test fixtures. A pipe ('|') would
// collide with Jinja's filter operator, so an otherwise-unused symbol is
// used instead.
const caret = "‸"

// offsetOf strips the caret marker from marked and returns the source runes
// plus the cursor offset it marked.
func offsetOf(t *testing.T, marked string) ([]rune, int) {
	t.Helper()
	i := strings.Index(marked, caret)
	if i < 0 {
		t.Fatalf("fixture has no caret marker: %q", marked)
	}
	stripped := marked[:i] + marked[i+len(caret):]
	return []rune(stripped), len([]rune(marked[:i]))
}

func TestScanToRegion(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want Region
	}{
		{"plain sql", "select ‸", RegionText},
		{"open expression", "{{ clean_‸", RegionExpr},
		{"closed expression", "{{ x }} ‸", RegionText},
		{"open statement", "{% set x = ‸", RegionStmt},
		{"jinja comment", "{# note ‸", RegionComment},
		{"raw block", "{% raw %}{{ f‸", RegionRaw},
		{"raw closed", "{% raw %}{{x}}{% endraw %}‸", RegionText},
		{"brace inside string", `{{ f("}}", g‸`, RegionExpr},
		{"escaped quote", `{{ f('it\'s', g‸`, RegionExpr},
		{"whitespace control", "{{- clean_‸", RegionExpr},

		// SQL quoting and SQL comments are transparent to Jinja: dbt
		// renders the template as plain text before it ever reaches SQL.
		{"jinja in sql string", "select '{{ g‸", RegionExpr},
		{"jinja in sql comment", "-- {{ g‸", RegionExpr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, cursor := offsetOf(t, tc.src)
			got := ScanTo(src, cursor)
			if got.Region != tc.want {
				t.Fatalf("ScanTo(%q) region = %v, want %v", tc.src, got.Region, tc.want)
			}
		})
	}
}

func TestScanToQuoteAndStack(t *testing.T) {
	src, cursor := offsetOf(t, `{{ outer_macro(inner("a,b", c‸`)
	st := ScanTo(src, cursor)

	if st.Region != RegionExpr {
		t.Fatalf("region = %v, want RegionExpr", st.Region)
	}
	if st.Quote != 0 {
		t.Fatalf("quote = %q, want 0 (string closed)", st.Quote)
	}
	if len(st.Stack) != 2 {
		t.Fatalf("stack depth = %d, want 2", len(st.Stack))
	}
	if st.Stack[0].callee != "outer_macro" {
		t.Fatalf("stack[0].callee = %q, want outer_macro", st.Stack[0].callee)
	}
	if st.Stack[1].callee != "inner" {
		t.Fatalf("stack[1].callee = %q, want inner", st.Stack[1].callee)
	}
	if st.Stack[1].argIndex != 1 {
		t.Fatalf("stack[1].argIndex = %d, want 1", st.Stack[1].argIndex)
	}
}

func TestScanToNoPanicOnTruncatedInput(t *testing.T) {
	fixtures := []string{
		"{{", "{%", "{#", "{% raw", "{{ f(", `{{ f("`, `{{ f('\`,
		"{%-", "{{-", "", "{", "{% raw %}{{ f",
	}
	for _, f := range fixtures {
		src := []rune(f)
		for cursor := 0; cursor <= len(src); cursor++ {
			_ = ScanTo(src, cursor)
		}
	}
}

func FuzzScanTo(f *testing.F) {
	seeds := []string{
		"{{ ref('a') }}",
		"{% set x = 1 %}",
		"{# c #}",
		"{% raw %}{{ x }}{% endraw %}",
		`{{ f("}}", g }}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		src := []rune(s)
		for cursor := 0; cursor <= len(src); cursor++ {
			_ = ScanTo(src, cursor)
		}
	})
}
