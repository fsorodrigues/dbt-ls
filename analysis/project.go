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
	if project.MacroPaths == nil {
		project.MacroPaths = []string{"macros"}
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
	s.setServerActive(false)
	s.disableProjectCapabilities()
	s.resetProjectState()
	s.ProjectMu.Lock()
	s.ProjectConfigPath = ""
	s.ProjectMu.Unlock()
	s.NotifyProject(message)
}

func (s *State) activateProject(root string, project DbtProject) error {
	s.resetProjectState()
	s.SetProjectRoot(root)
	var activationErr error

	if err := s.SetModelRoots(project.ModelPaths); err != nil {
		s.NotifyProject(fmt.Sprintf("dbt-ls: model functionality unavailable: %s", err))
		activationErr = err
	} else if err := s.ScanModelRoots(root); err != nil {
		s.setRefsEnabled(false)
		s.setDefinitionsEnabled(false)
		activationErr = err
	} else {
		s.setRefsEnabled(true)
		s.setDefinitionsEnabled(true)
	}

	if err := s.SetMacroRoots(project.MacroPaths); err != nil {
		s.NotifyProject(fmt.Sprintf("dbt-ls: macro functionality unavailable: %s", err))
		if activationErr == nil {
			activationErr = err
		}
	} else {
		s.setMacrosEnabled()
	}

	if err := s.ScanConfigRoot(root); err != nil {
		s.deactivateProject(root, fmt.Sprintf("dbt-ls: project could not be indexed: %s", err))
		return err
	}
	s.setServerActive(true)
	return activationErr
}

func (s *State) ActivateProject(root string, project DbtProject) error {
	return s.activateProject(root, project)
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
	if err := s.activateProject(root, project); err != nil {
		return
	}
	s.Logger.Tracef("dbt project activated: %s", configPath)
}
