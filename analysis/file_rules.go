package analysis

import (
	"path/filepath"
	"slices"
	"strings"
)

var (
	SKIPPABLE_DIR  []string = []string{"dbt_packages", "target"} // positive match
	TEMP_ARTIFACTS []string = []string{"4913"}                   // positive match
)

type DirEntry interface {
	IsDir() bool
}

func (s *State) isSkippableDir(path string, d DirEntry) bool {
	if d.IsDir() && slices.Contains(SKIPPABLE_DIR, path) {
		return true
	}
	return false
}

func (s *State) isSkippableTempArtifact(path string) bool {
	return slices.Contains(TEMP_ARTIFACTS, path)
}

func (s *State) isModelFile(path string) bool {
	return filepath.Ext(path) == s.DbtModelExtension &&
		slices.ContainsFunc(s.ModelRoots, func(sub string) bool {
			return strings.Contains(path, sub)
		})
}

func (s *State) isMacroFile(path string) bool {
	return filepath.Ext(path) == s.DbtMacroExtension &&
		slices.ContainsFunc(s.MacroRoots, func(sub string) bool {
			return strings.Contains(path, sub)
		})
}

func (s *State) isConfigFile(path string) bool {
	return slices.Contains(s.DbtConfigExtensions, filepath.Ext(path))
}
