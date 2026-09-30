package dbt

import "strings"

// Arg is a single declared macro parameter. Default is the literal source text
// of the default expression, or "" when the parameter has no default.
type Arg struct {
	Name    string
	Default string
}

// Macro is a callable Jinja macro declared in a dbt project.
type Macro struct {
	Name    string
	Package string
	File    string
	Line    int
	Args    []Arg
	Doc     string
	Builtin bool
}

func (m Macro) Signature() string {
	args := make([]string, 0, len(m.Args))
	for _, arg := range m.Args {
		value := arg.Name
		if arg.Default != "" {
			value += "=" + arg.Default
		}
		args = append(args, value)
	}
	return m.Name + "(" + strings.Join(args, ", ") + ")"
}
