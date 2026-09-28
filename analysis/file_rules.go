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

func (s *State) isUnderRoots(path string, roots []string) bool {
	abs := filepath.Clean(path)
	for _, root := range roots {
		full := filepath.Clean(filepath.Join(s.ProjectRoot, root))
		if rel, err := filepath.Rel(full, abs); err == nil &&
			rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return true
		}
	}
	return false
}

func (s *State) isModelFile(path string) bool {
	return filepath.Ext(path) == s.DbtModelExtension && s.isUnderRoots(path, s.ModelRoots)
}

func (s *State) isMacroFile(path string) bool {
	return filepath.Ext(path) == s.DbtMacroExtension && s.isUnderRoots(path, s.MacroRoots)
}

func (s *State) isConfigFile(path string) bool {
	return slices.Contains(s.DbtConfigExtensions, filepath.Ext(path))
}
