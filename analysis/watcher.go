package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
)

type DbtWatcher struct {
	Watcher *fsnotify.Watcher
	Root    string
	Type    string
}

func (w *DbtWatcher) Close() {
	w.Watcher.Close()
}

func (w *DbtWatcher) HandleAsyncClose(logger *log.Logger) {
	if w != nil {
		logger.Debug("Closing file watcher in HandleAsyncClose")
		w.Close()
	}
}

func NewWatcher(t, root string, logger *log.Logger) (*DbtWatcher, error) {
	watcher := &DbtWatcher{
		Root: root,
		Type: t,
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("Error creating an fsnotify.Watcher for %s. %s", t, err)
		return watcher, err
	}

	watcher.Watcher = fsWatcher
	return watcher, nil
}

func (s *State) WatchModels() {
	for {
		select {
		case event := <-s.ModelWatcher.Watcher.Events:
			if !s.ServerActive {
				continue
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				s.Logger.Infof("ModelWatcher Create event: %s", event.Name)
				info, err := os.Stat(event.Name)
				if filepath.Base(event.Name) == "4913" || (err == nil && !info.IsDir() && filepath.Ext(event.Name) != s.DbtModelExtension) {
					s.Logger.Debugf("ModelWatcher: ignoring event %s", event.Name)
					// handle special error cases that have to do with nvim way of saving
					// files this ignore when the file is name 4913 or when a file does
					// not have the correct extension.
					continue
				} else if err == nil && info.IsDir() && filepath.Base(event.Name) != "dbt_packages" {
					// New directory: scan and watch recursively
					s.Logger.Debugf("ModelWatcher: Found a new directory %s. Scanning it recursively.", event.Name)
					s.ScanAndWatchDirs([]string{event.Name}, s.FindModelFilesRecursive)
				} else if filepath.Ext(event.Name) == s.DbtModelExtension {
					s.Logger.Debugf("ModelWatcher: Found a new file %s", event.Name)
					s.AddNewModelToIndex(event.Name)
				} else if err != nil {
					s.Logger.Errorf("ModelWatcher: Error in Create event: %s", err)
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove && filepath.Ext(event.Name) == s.DbtModelExtension {
				s.Logger.Debugf("ModelWatcher Deletion Event %s", event.Name)
				if filepath.Ext(event.Name) == s.DbtModelExtension {
					s.RemoveModelFromIndex(event.Name)
				}
			}
			if event.Op&fsnotify.Rename == fsnotify.Rename && filepath.Ext(event.Name) == s.DbtModelExtension {
				s.Logger.Debugf("ModelWatcher Renaming Event %s", event.Name)
				if filepath.Ext(event.Name) == s.DbtModelExtension {
					s.RemoveModelFromIndex(event.Name)
				}
			}

		case err := <-s.ModelWatcher.Watcher.Errors:
			if !s.ServerActive {
				continue
			}
			s.Logger.Errorf("ModelWatcher error: %s", err.Error())
			s.Logger.Info("ModelWatcher: Attempting to restart model watcher functiontionality.")
			s.ModelWatcher.Watcher.Close()

			newModelWatcher, err := fsnotify.NewWatcher()
			if err != nil {
				s.Logger.Errorf("ModelWatcher: Error restarting the ModelWatcher. Stopping the model watcher functionality. %s", err)
				s.ModelWatcher.Watcher = nil
				break
			}
			s.ModelWatcher.Watcher = newModelWatcher
			for _, path := range s.RootPaths {
				modelDir := filepath.Join(path, s.ModelWatcher.Root)
				s.ScanAndWatchDirs([]string{modelDir}, s.FindModelFilesRecursive)
			}
			s.Logger.Info("ModelWatcher restarted succesfully")
		}
	}
}

func (s *State) WatchConfig() {
	for {
		select {
		case event := <-s.ConfigWatcher.Watcher.Events:
			if !s.ServerActive {
				continue
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				s.Logger.Infof("ConfigWatcher Create event: %s", event.Name)
				info, err := os.Stat(event.Name)
				if filepath.Base(event.Name) == "4913" || (err == nil && !info.IsDir() && !slices.Contains(s.DbtConfigExtensions, filepath.Ext(event.Name))) {
					s.Logger.Debugf("ConfigWatcher: ignoring event %s", event.Name)
					// handle special error cases that have to do with nvim way of saving
					// files this ignore when the file is name 4913 or when a file does
					// not have the correct extension.
					continue
				} else if err == nil && info.IsDir() && filepath.Base(event.Name) != "dbt_packages" {
					// New directory: scan and watch recursively
					s.Logger.Debugf("ConfigWatcher: Found a new directory %s. Scanning it recursively.", event.Name)
					s.ScanAndWatchDirs([]string{event.Name}, s.FindModelFilesRecursive)
				} else if slices.Contains(s.DbtConfigExtensions, filepath.Ext(event.Name)) {
					s.Logger.Debugf("ConfigWatcher: Found a new file %s", event.Name)
					s.ProcessNewConfigYaml(event.Name)
				} else if err != nil {
					s.Logger.Errorf("ConfigWatcher: Error in Create event: %s", err)
					s.RemoveConfigYaml(event.Name)
				}
			}

			if event.Op&fsnotify.Remove == fsnotify.Remove && slices.Contains(s.DbtConfigExtensions, filepath.Ext(event.Name)) {
				s.Logger.Infof("ConfigWatcher Deletion event: %s", event.Name)
			}

			if event.Op&fsnotify.Write == fsnotify.Write && slices.Contains(s.DbtConfigExtensions, filepath.Ext(event.Name)) {
				s.Logger.Infof("ConfigWatcher Write event: %s", event.Name)
				s.ProcessNewConfigYaml(event.Name)
			}

			if event.Op&fsnotify.Rename == fsnotify.Rename && slices.Contains(s.DbtConfigExtensions, filepath.Ext(event.Name)) {
				s.Logger.Debugf("ConfigWatcher Renaming Event %s", event.Name)
				_ = fmt.Sprintf("%s/models", "s")
			}
		case err := <-s.ConfigWatcher.Watcher.Errors:
			if !s.ServerActive {
				continue
			}
			s.Logger.Errorf("ConfigWatcher error: %s", err.Error())
			s.Logger.Info("ConfigWatcher: Attempting to restart ConfigWatcher functiontionality.")
			s.ConfigWatcher.Watcher.Close()

			newModelWatcher, err := fsnotify.NewWatcher()
			if err != nil {
				s.Logger.Errorf("ConfigWatcher: Error restarting the ConfigWatcher. Stopping the ConfigWatcher functionality. %s", err)
				s.ConfigWatcher.Watcher = nil
				break
			}
			s.ConfigWatcher.Watcher = newModelWatcher
			for _, path := range s.RootPaths {
				configDir := filepath.Join(path, s.ConfigWatcher.Root)
				s.ScanAndWatchDirs([]string{configDir}, s.FindConfigFilesRecursive)
			}
			s.Logger.Info("ConfigWatcher restarted succesfully")
		}
	}
}
