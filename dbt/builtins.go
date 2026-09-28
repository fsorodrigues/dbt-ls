package dbt

import "strings"

// Builtins are the dbt Jinja context members. They have no declaring file,
// so Builtin is true and File/Line are empty.
//
// Source: https://docs.getdbt.com/reference/dbt-jinja-functions
var Builtins = []Macro{
	{Name: "ref", Builtin: true, Args: []Arg{{Name: "model_name"}},
		Doc: "Reference another model in the project."},
	{Name: "source", Builtin: true, Args: []Arg{{Name: "source_name"}, {Name: "table_name"}},
		Doc: "Reference a table declared in a sources YAML file."},
	{Name: "config", Builtin: true,
		Doc: "Set model configuration from within the model."},
	{Name: "var", Builtin: true, Args: []Arg{{Name: "name"}, {Name: "default"}}},
	{Name: "env_var", Builtin: true, Args: []Arg{{Name: "name"}, {Name: "default"}}},
	{Name: "is_incremental", Builtin: true},
	{Name: "this", Builtin: true},
	{Name: "target", Builtin: true},
	{Name: "log", Builtin: true, Args: []Arg{{Name: "msg"}, {Name: "info", Default: "False"}}},
	{Name: "run_query", Builtin: true, Args: []Arg{{Name: "sql"}}},
	{Name: "statement", Builtin: true},
	{Name: "return", Builtin: true, Args: []Arg{{Name: "value"}}},
	{Name: "adapter", Builtin: true},
	{Name: "exceptions", Builtin: true},
	{Name: "modules", Builtin: true},
	{Name: "graph", Builtin: true},
	{Name: "invocation_id", Builtin: true},
	{Name: "run_started_at", Builtin: true},
	{Name: "dbt_version", Builtin: true},
	{Name: "flags", Builtin: true},
	{Name: "print", Builtin: true, Args: []Arg{{Name: "msg"}}},
	{Name: "tojson", Builtin: true}, {Name: "fromjson", Builtin: true},
	{Name: "toyaml", Builtin: true}, {Name: "fromyaml", Builtin: true},
	{Name: "load_result", Builtin: true, Args: []Arg{{Name: "name"}}},
	{Name: "selected_resources", Builtin: true},
	{Name: "builtins", Builtin: true},
}

// BuiltinsWithPrefix returns every builtin whose name starts with prefix,
// case-insensitively. An empty prefix matches everything.
func BuiltinsWithPrefix(prefix string) []Macro {
	prefix = strings.ToLower(prefix)

	matches := make([]Macro, 0, len(Builtins))
	for _, m := range Builtins {
		if strings.HasPrefix(strings.ToLower(m.Name), prefix) {
			matches = append(matches, m)
		}
	}
	return matches
}
