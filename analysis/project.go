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
