package analysis

import (
	"path/filepath"
	"strings"
)

func (s *State) AddNewModelToIndex(file string) {
	s.DbtModelsMu.Lock()
	defer s.DbtModelsMu.Unlock()
	s.Logger.Debugf("Adding file: %s", file)
	s.DbtModels.Put(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtModelExtension),
		file,
	)
}

func (s *State) RemoveModelFromIndex(file string) {
	s.DbtModelsMu.Lock()
	defer s.DbtModelsMu.Unlock()
	s.Logger.Debugf("Removing file: %s", file)
	s.DbtModels.Remove(
		strings.TrimSuffix(strings.ToLower(filepath.Base(file)), s.DbtModelExtension),
	)
}
