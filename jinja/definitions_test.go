package jinja

import (
	"reflect"
	"testing"
)

func TestDefinitions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []Definition
	}{
		{
			name: "simple macro with doc",
			src: `
				{#- Converts an integer cents column to dollars. -#}
				{% macro cents_to_dollars(column_name, precision=2) -%}
						round({{ column_name }} / 100, {{ precision }})
				{%- endmacro %}
			`,
			want: []Definition{
				{
					Kind: DefMacro,
					Name: "cents_to_dollars",
					Line: 2,
					Params: []Param{
						{Name: "column_name"},
						{Name: "precision", Default: "2"},
					},
					Doc: "Converts an integer cents column to dollars.",
				},
			},
		},
		{
			name: "no args, no doc",
			src: `
				{% macro current_timestamp() %}
					current_timestamp()
				{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "current_timestamp",
					Line:   1,
					Params: []Param{},
				},
			},
		},
		{
			name: "multiple macros preserve source order",
			src: `
				{% macro dollars_to_cents(column_name) %}
					{{ column_name }} * 100
				{% endmacro %}

				{% macro cents_to_dollars(column_name) %}
					{{ column_name }} / 100
				{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "dollars_to_cents",
					Line:   1,
					Params: []Param{{Name: "column_name"}},
				},
				{
					Kind:   DefMacro,
					Name:   "cents_to_dollars",
					Line:   5,
					Params: []Param{{Name: "column_name"}},
				},
			},
		},
		{
			name: "default containing a comma and nested call",
			src: `
				{% macro star(relation, except=[]) %}
					select * from {{ relation }}
				{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "star",
					Line:   1,
					Params: []Param{{Name: "relation"}, {Name: "except", Default: "[]"}},
				},
			},
		},
		{
			name: "doc comment separated by other text does not attach",
			src: `
				{#- Not adjacent. -#}
				select 1;
				{% macro foo(x) %}{{ x }}{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "foo",
					Line:   3,
					Params: []Param{{Name: "x"}},
					Doc:    "",
				},
			},
		},
		{
			name: "set statement is not a declaration",
			src: `
				{% set x = 1 %}
				{% macro foo(x) %}{{ x }}{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "foo",
					Line:   2,
					Params: []Param{{Name: "x"}},
				},
			},
		},
		{
			name: "macro-like text inside a comment is ignored",
			src: `
				{# {% macro fake(a) %}{% endmacro %} #}
				{% macro real(a) %}{{ a }}{% endmacro %}
			`,
			want: []Definition{
				{
					Kind:   DefMacro,
					Name:   "real",
					Line:   2,
					Params: []Param{{Name: "a"}},
					// The comment happens to be adjacent, so its (odd)
					// contents become the doc string. The important
					// assertion here is that "fake" was never parsed as
					// its own Definition.
					Doc: "{% macro fake(a) %}{% endmacro %}",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Definitions([]rune(tc.src))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Definitions() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseDefinitionTag(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want Definition
		ok   bool
	}{
		{
			name: "macro with defaults",
			tag:  "{% macro cents_to_dollars(column_name, precision=2) -%}",
			want: Definition{
				Kind: DefMacro,
				Name: "cents_to_dollars",
				Params: []Param{
					{Name: "column_name"},
					{Name: "precision", Default: "2"},
				},
			},
			ok: true,
		},
		{
			name: "set is not a declaration",
			tag:  "{% set x = 1 %}",
			ok:   false,
		},
		{
			name: "endmacro is not a declaration",
			tag:  "{% endmacro %}",
			ok:   false,
		},
		{
			name: "materialization without parens",
			tag:  "{% materialization my_materialization, adapter='snowflake' %}",
			want: Definition{
				Kind:   DefMaterialization,
				Name:   "my_materialization",
				Params: []Param{{Name: "adapter", Default: "'snowflake'"}},
			},
			ok: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseDefinitionTag([]rune(tc.tag), 0)
			if ok != tc.ok {
				t.Fatalf("parseDefinitionTag(%q) ok = %v, want %v", tc.tag, ok, tc.ok)
			}
			if !ok {
				return
			}
			tc.want.Line = 0
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseDefinitionTag(%q) = %+v, want %+v", tc.tag, got, tc.want)
			}
		})
	}
}
