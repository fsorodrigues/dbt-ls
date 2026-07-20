package analysis

type DbtConfig struct {
	Name    string
	Sources map[string]*DbtConfigSource
}

type DbtConfigSource struct {
	Name     string
	Database string
	Schema   string
	Tables   map[string]*DbtTable
}

type DbtTable struct {
	Name       string `yaml:"name"`
	Identifier string `yaml:"identifier"`
	SourceFile string
}

type DbtSource struct {
	Name     string      `yaml:"name"`
	Database string      `yaml:"database"`
	Schema   string      `yaml:"schema"`
	Tables   []*DbtTable `yaml:"tables"`
}

type DbtSources struct {
	Sources []*DbtSource `yaml:"sources"`
}

type DbtProject struct {
	Name       string   `yaml:"name"`
	ModelPaths []string `yaml:"model-paths"`
	DbtSources
}
