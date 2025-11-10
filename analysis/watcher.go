package analysis

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func (s *State) WatchProject() {
	for {
		select {
		case event := <-s.Watcher.Events:
			if event.Op&fsnotify.Create == fsnotify.Create {
				s.Logger.Infof("Watcher Create event: %s", event.Name)
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					// New directory: scan and watch recursively
					s.Logger.Debugf("Found a new directory %s. Scanning it recursively.", event.Name)
					s.ScanAndWatchDirs([]string{event.Name})
				} else if filepath.Ext(event.Name) == ".r" {
					s.Logger.Debugf("Found a new file %s", event.Name)
					// need to add function that adds model to index. this will be a
					// refactored function that does what lines 108-109 in s.go does
					s.AddNewModelToIndex(event.Name)
				} else if err != nil {
					s.Logger.Errorf("Error in Create event: %s", err)
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				s.Logger.Debugf("Deletion Event %s", event.Name)
				if filepath.Ext(event.Name) == s.DbtModelExtension {
					s.RemoveModelFromIndex(event.Name)
				}
			}
			if event.Op&fsnotify.Rename == fsnotify.Rename {
				s.Logger.Debugf("Renaming Event %s", event.Name)
			}

		case err := <-s.Watcher.Errors:
			s.Logger.Errorf("Watcher error: %s", err.Error())
			s.Logger.Info("Attempting to restart the LSP")
			s.Watcher.Close()

			newWatcher, err := fsnotify.NewWatcher()
			if err != nil {
				s.Logger.Errorf("Error restarting the Watcher. Stopping the watcher functionality. %s", err)
				s.Watcher = nil
				break
			}
			s.Watcher = newWatcher
			roots := make([]string, 0, len(s.Root))
			for _, r := range s.Root {
				dir := fmt.Sprintf("%s/models", r.Name)
				roots = append(roots, dir)
			}
			s.ScanAndWatchDirs(roots)
			s.Logger.Info("Watcher restarted succesfully")
		}
	}
}
