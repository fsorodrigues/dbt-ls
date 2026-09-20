package dbt

type Macro struct {
	filePath string
}

func NewMacro(filePath string) Macro {
	return Macro{
		filePath: filePath,
	}
}
