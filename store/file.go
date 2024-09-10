package store

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"sync"

// 	"github.com/fsnotify/fsnotify"
// 	log "github.com/sirupsen/logrus"
// )

// type FileScriptStore struct {
// 	filePath string
// 	mu       sync.Mutex
// 	scripts  *sync.Map
// }

// func NewFileScriptStore(filePath string) (*FileScriptStore, error) {
// 	scripts, err := loadScriptsFromFile(filePath)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &FileScriptStore{
// 		filePath: filePath,
// 		scripts:  scripts,
// 	}, nil
// }

// func (f *FileScriptStore) LoadInitialScripts() (*sync.Map, error) {
// 	return f.scripts, nil
// }

// // WatchScripts uses fsnotify to monitor the script file for changes
// func (f *FileScriptStore) WatchScripts(ctx context.Context, onChange func(subject, path, script string, deleted bool)) {
// 	watcher, err := fsnotify.NewWatcher()
// 	if err != nil {
// 		log.Fatalf("failed to create watcher: %v", err)
// 	}
// 	defer watcher.Close()

// 	// Add the file to the watcher
// 	err = watcher.Add(f.filePath)
// 	if err != nil {
// 		log.Fatalf("failed to add file to watcher: %v", err)
// 	}

// 	log.Infof("Started watching file: %s", f.filePath)

// 	for {
// 		select {
// 		case event, ok := <-watcher.Events:
// 			if !ok {
// 				return
// 			}

// 			// We only care about write events, which means the file has been modified
// 			if event.Op&fsnotify.Write == fsnotify.Write {
// 				log.Infof("File modified: %s", f.filePath)

// 				// Reload the scripts from the file
// 				f.mu.Lock()
// 				scripts, err := loadScriptsFromFile(f.filePath)
// 				if err != nil {
// 					log.Errorf("Failed to reload scripts: %v", err)
// 					f.mu.Unlock()
// 					continue
// 				}
// 				f.scripts = scripts
// 				f.mu.Unlock()

// 				// Trigger the onChange callback for each script
// 				scripts.Range(func(subject, script) {
// 					onChange(subject, f.filePath, script, false)
// 					return true
// 				})
// 			}

// 		case err, ok := <-watcher.Errors:
// 			if !ok {
// 				return
// 			}
// 			log.Errorf("Watcher error: %v", err)
// 		case <-ctx.Done():
// 			log.Info("Stopping watcher")
// 			return
// 		}
// 	}
// }

// func (f *FileScriptStore) GetScript(subject string) (string, error) {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	script, ok := f.scripts.Load(subject)
// 	if !ok {
// 		return "", fmt.Errorf("script not found for subject: %s", subject)
// 	}
// 	return script.(string), nil
// }

// func loadScriptsFromFile(filePath string) (*sync.Map, error) {
// 	// This function reads the scripts from a JSON file and returns a map.
// 	fileData, err := os.ReadFile(filePath)
// 	if err != nil {
// 		return nil, err
// 	}

// 	scripts := new(sync.Map)
// 	if err := json.Unmarshal(fileData, &scripts); err != nil {
// 		return nil, err
// 	}

// 	return scripts, nil
// }
