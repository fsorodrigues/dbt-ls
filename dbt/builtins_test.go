package dbt

import "testing"

func TestBuiltinsWithPrefix(t *testing.T) {
	got := BuiltinsWithPrefix("re")

	want := map[string]bool{"ref": true, "return": true}
	if len(got) != len(want) {
		t.Fatalf("BuiltinsWithPrefix(\"re\") = %v, want names %v", got, want)
	}
	for _, m := range got {
		if !want[m.Name] {
			t.Errorf("unexpected match %q", m.Name)
		}
		if !m.Builtin {
			t.Errorf("%q should be marked Builtin", m.Name)
		}
	}
}

func TestBuiltinsWithPrefixCaseInsensitive(t *testing.T) {
	got := BuiltinsWithPrefix("REF")
	if len(got) != 1 || got[0].Name != "ref" {
		t.Fatalf("BuiltinsWithPrefix(\"REF\") = %v, want [ref]", got)
	}
}
