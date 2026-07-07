package analysis

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/fsnotify/fsnotify"
)

type DbtWatcher struct {
	Watcher *fsnotify.Watcher
	Root    string
	Type    string
}

func (w *DbtWatcher) Close() {
	if w == nil || w.Watcher == nil {
		return
	}
	_ = w.Watcher.Close()
}

func (w *DbtWatcher) HandleAsyncClose(logger *log.Logger) {
	if w != nil {
		logger.Debug("Closing ProjectWatcher in HandleAsyncClose")
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

func (s *State) handleCreateEvent(path string) {
	s.Logger.Infof("[WatchProject]: Create event: %s", path)
	info, err := os.Stat(path)
	if err != nil {
		s.Logger.Errorf("[WatchProject]: Error in Create event: %s", err)
		return
	}

	base := filepath.Base(path)
	if s.isSkippableTempArtifact(base) {
		s.Logger.Debugf("[WatchProject]: ignoring temp artifact %s", path)
		return
	}

	if info.IsDir() {
		if s.isSkippableDir(base, info) {
			s.Logger.Debugf("[WatchProject]: ignoring skippable dir %s", path)
			return
		}
		s.Logger.Debugf("[WatchProject]: Found new directory %s. Scanning recursively.", path)
		if err := s.ScanProjectFiles(path); err != nil {
			s.Logger.Errorf("[WatchProject]: Error scanning new directory %s: %s", path, err)
		}
		return
	}

	if s.isConfigFile(base) {
		s.Logger.Debugf("[WatchProject]: Found new config file %s", path)
		s.ProcessNewConfigYaml(path)
		return
	}
	if s.isModelFile(base) {
		s.Logger.Debugf("[WatchProject]: Found new model file %s", path)
		s.AddNewModelToIndex(path)
		return
	}

	s.Logger.Debugf("[WatchProject]: ignoring unsupported create event %s", path)
}

func (s *State) restartProjectWatcher() {
	s.Logger.Info("[WatchProject]: Attempting to restart ProjectWatcher functionality.")
	s.ProjectWatcher.Close()

	newWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.Logger.Errorf(
			"[WatchProject]: Error restarting ProjectWatcher. Stopping watcher functionality. %s",
			err,
		)
		s.ProjectWatcher.Watcher = nil
		return
	}

	s.ProjectWatcher.Watcher = newWatcher
	s.resetWatchedDirs()
	if err := s.ScanWorkspaceRoots(); err != nil {
		s.Logger.Errorf("[WatchProject]: Error rescanning workspace roots after restart: %s", err)
		return
	}
	s.Logger.Info("[WatchProject]: restarted successfully")
}

func (s *State) WatchProject() {
	for {
		select {
		case event := <-s.ProjectWatcher.Watcher.Events:
			if !s.ServerActive {
				continue
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				s.handleCreateEvent(event.Name)
			}

			base := filepath.Base(event.Name)
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				if s.isConfigFile(base) {
					s.Logger.Infof("[WatchProject]: Config Deletion event: %s", event.Name)
					s.RemoveConfigYaml(event.Name)
				} else if s.isModelFile(base) {
					s.Logger.Infof("[WatchProject]: Model Deletion event: %s", event.Name)
					s.RemoveModelFromIndex(event.Name)
				}
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				if s.isConfigFile(base) {
					s.Logger.Infof("[WatchProject]: Config Write event: %s", event.Name)
					s.ProcessNewConfigYaml(event.Name)
				} else if s.isModelFile(base) {
					s.Logger.Infof("[WatchProject]: Model Write event: %s", event.Name)
					// do nothing
					continue
				}
			}

			if event.Op&fsnotify.Rename == fsnotify.Rename {
				if s.isConfigFile(base) {
					s.Logger.Debugf("[WatchProject]: Config Renaming Event %s", event.Name)
					// handle case
					s.RemoveConfigYaml(event.Name)
				} else if s.isModelFile(base) {
					s.Logger.Debugf("[WatchProject]: Model Renaming Event %s", event.Name)
					s.RemoveModelFromIndex(event.Name)
				}
			}

		case err := <-s.ProjectWatcher.Watcher.Errors:
			if !s.ServerActive {
				continue
			}
			s.Logger.Errorf("[WatchProject]: error: %s", err.Error())
			s.restartProjectWatcher()
		}
	}
}
