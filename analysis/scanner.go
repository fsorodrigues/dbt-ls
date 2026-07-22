package analysis

import (
	"io/fs"
	"path/filepath"
	"slices"
)

type ScanCallback func(string) error

func (s *State) addWatchDir(path string) error {
	if s.ProjectWatcher == nil || s.ProjectWatcher.Watcher == nil {
		return nil
	}

	s.watchedDirsMu.Lock()
	defer s.watchedDirsMu.Unlock()
	if _, ok := s.watchedDirs[path]; ok {
		return nil
	}
	if err := s.ProjectWatcher.Watcher.Add(path); err != nil {
		return err
	}
	s.watchedDirs[path] = struct{}{}
	return nil
}

func (s *State) AddWatchDir(path string) error {
	return s.addWatchDir(path)
}

func (s *State) resetWatchedDirs() {
	s.watchedDirsMu.Lock()
	defer s.watchedDirsMu.Unlock()
	s.watchedDirs = map[string]struct{}{}
}

func (s *State) findFilesRecursive(root string, exts []string) ([]string, error) {
	s.Logger.Debugf("Starting recursive search on %s", root)
	var matchingFiles []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		s.Logger.Tracef("Looking at %s", path)
		if err != nil {
			s.Logger.Errorf("Error while looking at %s: %s", path, err)
			return err // Handle errors during traversal
		}
		base := filepath.Base(path)
		if s.isSkippableDir(base, d) {
			s.Logger.Tracef("Ignoring %s branch of the tree", base)
			return fs.SkipDir
		}
		if d.IsDir() {
			s.Logger.Tracef("Found dir %s. Adding to ProjectWatcher", path)
			if err := s.addWatchDir(path); err != nil {
				s.Logger.Errorf("Error adding %s to ProjectWatcher: %s", path, err)
			}
		} else if slices.Contains(exts, filepath.Ext(path)) {
			s.Logger.Tracef("Found %s file %s. Selected for LSP indexing", filepath.Ext(path), path)
			matchingFiles = append(matchingFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matchingFiles, nil
}

func (s *State) FindModelFilesRecursive(dirPattern string) error {
	s.Logger.Infof(
		"Starting scan. Looking for models with extension %s",
		s.DbtModelExtension,
	)

	models, err := s.findFilesRecursive(dirPattern, []string{s.DbtModelExtension})
	if err != nil {
		s.Logger.Errorf("Error finding model files recursively: %s", err)
		return err
	}

	s.Logger.Debugf("Adding %d models to state", len(models))
	for _, file := range models {
		s.AddNewModelToIndex(file)
	}
	return nil
}

func (s *State) FindConfigFilesRecursive(dirPattern string) error {
	s.Logger.Infof(
		"Starting scan. Looking for configs with extensions %s",
		s.DbtConfigExtensions,
	)
	configs, err := s.findFilesRecursive(dirPattern, s.DbtConfigExtensions)
	if err != nil {
		s.Logger.Errorf("Error finding config files recursively: %s", err)
		return err
	}

	s.Logger.Debugf("Adding %d configs to state", len(configs))
	for _, file := range configs {
		s.ProcessNewConfigYaml(file)
	}
	return nil
}

func (s *State) ScanProjectFiles(root string) error {
	if err := s.FindModelFilesRecursive(root); err != nil {
		return err
	}
	return s.FindConfigFilesRecursive(root)
}

func (s *State) ScanRootPath(rootPath string) error {
	modelDirs := make([]string, 0, len(s.ModelRoots))
	for _, modelRoot := range s.ModelRoots {
		modelDirs = append(modelDirs, filepath.Join(rootPath, modelRoot))
	}
	if len(modelDirs) == 0 {
		s.Logger.Info("No model paths configured; skipping model scan")
	}
	if err := s.ScanAndWatchDirs(modelDirs, s.FindModelFilesRecursive); err != nil {
		return err
	}

	configDir := filepath.Join(rootPath, s.ConfigRoot)
	return s.ScanAndWatchDirs([]string{configDir}, s.FindConfigFilesRecursive)
}

func (s *State) ScanWorkspaceRoots() error {
	for _, rootPath := range s.RootPaths {
		if err := s.ScanRootPath(rootPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) ScanAndWatchDirs(roots []string, callback ScanCallback) error {
	for _, dir := range roots {
		s.Logger.Debugf("Scanning in: %s", dir)
		if err := callback(dir); err != nil {
			return err
		}
	}
	return nil
}
