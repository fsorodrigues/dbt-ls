package analysis

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

var ErrDbtProjectNotFound = errors.New("dbt project configuration not found")

func (s *State) ParseDbtConfig(rootPath string) (DbtProject, error) {
	var configPath string
	for _, ext := range s.DbtConfigExtensions {
		candidate := filepath.Join(rootPath, "dbt_project"+ext)
		s.Logger.Tracef("Testing for %s", candidate)
		if _, err := os.Stat(candidate); err == nil {
			configPath = candidate
			break
		} else if !os.IsNotExist(err) {
			return DbtProject{}, fmt.Errorf("stat dbt project configuration %s: %w", candidate, err)
		}
	}

	if configPath == "" {
		return DbtProject{}, fmt.Errorf("%w in %s", ErrDbtProjectNotFound, rootPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return DbtProject{}, fmt.Errorf("read dbt project configuration %s: %w", configPath, err)
	}

	var project DbtProject
	if err := yaml.Unmarshal(data, &project); err != nil {
		return DbtProject{}, fmt.Errorf("parse dbt project configuration %s: %w", configPath, err)
	}
	if project.ModelPaths == nil {
		project.ModelPaths = []string{"models"}
	}

	s.Logger.Debugf("Dbt project configuration found: %s", configPath)
	return project, nil
}

func (s *State) projectConfigPath(rootPath string) string {
	for _, ext := range s.DbtConfigExtensions {
		candidate := filepath.Join(rootPath, "dbt_project"+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (s *State) GetProjectConfigPath(rootPath string) string {
	return s.projectConfigPath(rootPath)
}

func (s *State) isProjectConfigFile(path string) bool {
	s.ProjectMu.RLock()
	root := s.ProjectRoot
	s.ProjectMu.RUnlock()
	if root == "" {
		return false
	}
	return filepath.Clean(path) == filepath.Clean(filepath.Join(root, "dbt_project.yml")) ||
		filepath.Clean(path) == filepath.Clean(filepath.Join(root, "dbt_project.yaml"))
}

func (s *State) deactivateProject(root, message string) {
	wasActive := s.IsServerActive()
	s.setServerActive(false)
	s.disableProjectCapabilities()
	s.resetProjectState()
	s.ProjectMu.Lock()
	s.ProjectConfigPath = ""
	s.ProjectMu.Unlock()
	if wasActive {
		s.NotifyProject(message)
	}
}

func (s *State) activateProject(root, configPath string, project DbtProject) error {
	s.resetProjectState()
	s.ModelRoots = project.ModelPaths
	s.ProjectMu.Lock()
	s.ProjectRoot = root
	s.ProjectConfigPath = configPath
	s.ProjectMu.Unlock()
	if err := s.ScanRootPath(root); err != nil {
		s.deactivateProject(root, fmt.Sprintf("dbt-ls: project could not be indexed: %s", err))
		return err
	}
	s.enableProjectCapabilities(s.sourcesEnabled())
	s.setServerActive(true)
	return nil
}

func (s *State) ActivateProject(root, configPath string, project DbtProject) error {
	return s.activateProject(root, configPath, project)
}

func (s *State) reconcileProject(root string) {
	s.ProjectLifecycleMu.Lock()
	defer s.ProjectLifecycleMu.Unlock()

	configPath := s.projectConfigPath(root)
	if configPath == "" {
		s.deactivateProject(root, "dbt-ls: project is now invalid; server deactivated")
		return
	}
	project, err := s.ParseDbtConfig(root)
	if err != nil {
		s.deactivateProject(root, "dbt-ls: project configuration is invalid; server deactivated")
		s.Logger.Errorf("Unable to reconcile dbt project: %s", err)
		return
	}
	if err := s.activateProject(root, configPath, project); err != nil {
		return
	}
	s.Logger.Tracef("dbt project activated: %s", configPath)
}
