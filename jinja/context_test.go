package jinja

import "testing"

func TestClassifyRole(t *testing.T) {
	cases := []struct {
		src      string
		role     Role
		prefix   string
		receiver string
	}{
		{"{{ dbt_ut‸", RoleValue, "dbt_ut", ""},
		{"{{ outer(inner(a, b‸", RoleValue, "b", ""},
		{"{% set x = cl‸", RoleValue, "cl", ""},
		{"{% set cl‸", RoleBindingName, "cl", ""},
		{"{% macro cl‸", RoleBindingName, "cl", ""},
		{"{% for ro‸", RoleBindingName, "ro", ""},
		{"{% if check_‸", RoleValue, "check_", ""},
		{"{% call cl‸", RoleValue, "cl", ""},
		{"{% do clean_‸", RoleValue, "clean_", ""},
		{"{% en‸", RoleStmtKeyword, "en", ""},
		{"{{ adapter.dis‸", RoleMember, "dis", "adapter"},
		{"{{ x is cu‸", RoleTest, "cu", ""},
		{"{{ x is not cu‸", RoleTest, "cu", ""},
		{"{{ y | up‸", RoleFilter, "up", ""},
		{"{# cl‸", RoleNone, "", ""},
		{"{% raw %}{{ f‸", RoleNone, "", ""},
		{"select ‸", RoleNone, "", ""},
		{`{{ f("}}", g‸`, RoleValue, "g", ""},
		{"select '{{ g‸", RoleValue, "g", ""},
		{"-- {{ g‸", RoleValue, "g", ""},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src, cursor := offsetOf(t, tc.src)
			ctx := Classify(src, cursor)

			if ctx.Role != tc.role {
				t.Fatalf("Classify(%q).Role = %v, want %v", tc.src, ctx.Role, tc.role)
			}
			if ctx.Prefix != tc.prefix {
				t.Fatalf("Classify(%q).Prefix = %q, want %q", tc.src, ctx.Prefix, tc.prefix)
			}
			if ctx.Receiver != tc.receiver {
				t.Fatalf("Classify(%q).Receiver = %q, want %q", tc.src, ctx.Receiver, tc.receiver)
			}
		})
	}
}

func TestClassifyStringArg(t *testing.T) {
	cases := []struct {
		src      string
		prefix   string
		callee   string
		argIndex int
	}{
		{"{{ ref('my_‸')", "my_", "ref", 0},
		{"{{ source('a', 'b‸')", "b", "source", 1},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src, cursor := offsetOf(t, tc.src)
			ctx := Classify(src, cursor)

			if ctx.Role != RoleStringArg {
				t.Fatalf("Classify(%q).Role = %v, want RoleStringArg", tc.src, ctx.Role)
			}
			if ctx.Prefix != tc.prefix {
				t.Fatalf("Classify(%q).Prefix = %q, want %q", tc.src, ctx.Prefix, tc.prefix)
			}
			if ctx.Callee != tc.callee {
				t.Fatalf("Classify(%q).Callee = %q, want %q", tc.src, ctx.Callee, tc.callee)
			}
			if ctx.ArgIndex != tc.argIndex {
				t.Fatalf("Classify(%q).ArgIndex = %d, want %d", tc.src, ctx.ArgIndex, tc.argIndex)
			}
		})
	}
}

func TestClassifyKwargName(t *testing.T) {
	src, cursor := offsetOf(t, "{{ star(fr‸=1)")
	ctx := Classify(src, cursor)

	if ctx.Role != RoleKwargName {
		t.Fatalf("Role = %v, want RoleKwargName", ctx.Role)
	}
	if ctx.Prefix != "fr" {
		t.Fatalf("Prefix = %q, want fr", ctx.Prefix)
	}
	if ctx.Callee != "star" {
		t.Fatalf("Callee = %q, want star", ctx.Callee)
	}
}

func TestClassifyMidWord(t *testing.T) {
	src, cursor := offsetOf(t, "{{ cle‸an_name }}")
	ctx := Classify(src, cursor)

	if ctx.Prefix != "cle" {
		t.Fatalf("Prefix = %q, want cle", ctx.Prefix)
	}
	if !(ctx.Start < cursor && cursor < ctx.End) {
		t.Fatalf("Start=%d cursor=%d End=%d, want Start < cursor < End", ctx.Start, cursor, ctx.End)
	}
	full := string(src[ctx.Start:ctx.End])
	if full != "clean_name" {
		t.Fatalf("full word = %q, want clean_name", full)
	}
}

func TestClassifySetVsSetEquals(t *testing.T) {
	// {% set clean_| %} is a binding name; {% set x = clean_| %} is a value
	// — the '=' flips it.
	src, cursor := offsetOf(t, "{% set clean_‸")
	if got := Classify(src, cursor).Role; got != RoleBindingName {
		t.Fatalf("Role = %v, want RoleBindingName", got)
	}

	src, cursor = offsetOf(t, "{% set x = clean_‸")
	if got := Classify(src, cursor).Role; got != RoleValue {
		t.Fatalf("Role = %v, want RoleValue", got)
	}
}

func TestClassifyForInFlips(t *testing.T) {
	// {% for clean_| %} is a binding name; once "in" has been seen it's a
	// value again (e.g. {% for x in clean_| %}).
	src, cursor := offsetOf(t, "{% for clean_‸")
	if got := Classify(src, cursor).Role; got != RoleBindingName {
		t.Fatalf("Role = %v, want RoleBindingName", got)
	}

	src, cursor = offsetOf(t, "{% for x in clean_‸")
	if got := Classify(src, cursor).Role; got != RoleValue {
		t.Fatalf("Role = %v, want RoleValue", got)
	}
}
