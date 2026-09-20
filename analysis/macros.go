package analysis

import (
	"path/filepath"
	"strings"

	"github.com/fsorodrigues/dbt-ls/dbt"
)

func (s *State) AddNewMacroToIndex(file string, macro dbt.Macro) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Adding file: %s", file)
	s.DbtMacros.Put(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtMacroExtension),
		macro,
	)
}

func (s *State) RemoveMacroFromIndex(file string) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Removing file: %s", file)
	s.DbtMacros.Remove(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtMacroExtension),
	)
}
