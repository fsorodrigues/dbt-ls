package analysis

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/fsorodrigues/dbt-ls/dbt"
	"github.com/fsorodrigues/dbt-ls/jinja"
)

func (s *State) AddNewMacroFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		s.Logger.Errorf("Error reading macro file %s: %s", path, err)
		return
	}
	if !utf8.Valid(src) {
		s.Logger.Errorf("Macro file is not valid UTF-8: %s", path)
		return
	}

	// Deduplicate: skip if the file content hasn't changed since last parse.
	// Neovim's atomic save fires several fsnotify events per save cycle; all of
	// them read identical bytes, so only the first one should proceed.
	name := filepath.Clean(path)
	hash := fmt.Sprintf("%x", sha256.Sum256(src))
	s.macroFileHashesMu.Lock()
	if s.macroFileHashes[name] == hash {
		s.macroFileHashesMu.Unlock()
		s.Logger.Tracef("Macro file %s unchanged (hash match). Skipping reparse.", path)
		return
	}
	s.macroFileHashesMu.Unlock()

	definitions := jinja.Definitions(bytes.Runes(src))
	macroNames := make([]string, 0, len(definitions))
	macros := make([]dbt.Macro, 0, len(definitions))
	for _, def := range definitions {
		if def.Kind != jinja.DefMacro {
			continue
		}

		macros = append(macros, dbt.Macro{
			Name: def.Name,
			File: filepath.Clean(path),
			Line: def.Line,
			Args: toArgs(def.Params),
			Doc:  def.Doc,
		})
		macroNames = append(macroNames, strings.ToLower(def.Name))
	}

	s.RemoveMacroFileFromIndex(path)

	for i, macro := range macros {
		s.AddNewMacroToIndex(macroNames[i], macro)
	}

	s.AddNewMacroFileToIndex(path, macroNames)

	s.macroFileHashesMu.Lock()
	s.macroFileHashes[name] = hash
	s.macroFileHashesMu.Unlock()
}

func toArgs(params []jinja.Param) []dbt.Arg {
	args := make([]dbt.Arg, 0, len(params))
	for _, p := range params {
		args = append(args, dbt.Arg{Name: p.Name, Default: p.Default})
	}
	return args
}

func (s *State) AddNewMacroFileToIndex(path string, macroNames []string) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Adding macro file: %s", path)

	name := filepath.Clean(path)
	s.DbtMacroFiles[name] = macroNames
}

func (s *State) RemoveMacroFileFromIndex(path string) {
	s.DbtMacrosMu.Lock()
	name := filepath.Clean(path)
	macros := s.DbtMacroFiles[name]

	for _, mac := range macros {
		s.DbtMacros.Remove(mac)
	}

	s.DbtMacroFiles[name] = nil
	s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Removing macro file: %s", path)

	s.macroFileHashesMu.Lock()
	delete(s.macroFileHashes, name)
	s.macroFileHashesMu.Unlock()
}

func (s *State) AddNewMacroToIndex(name string, macro dbt.Macro) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Adding macro: %s, %+v", name, macro)
	s.DbtMacros.Put(name, macro)
}

func (s *State) RemoveMacroFromIndex(name string) {
	s.DbtMacrosMu.Lock()
	defer s.DbtMacrosMu.Unlock()
	s.Logger.Tracef("Removing macro: %s", name)
	s.DbtMacros.Remove(name)
}
