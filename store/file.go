package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/numkem/msgscript"
	log "github.com/sirupsen/logrus"
)

type FileScriptStore struct {
	filePath string
	scripts  *sync.Map
	libs     *sync.Map
}

func NewFileScriptStore(scriptPath string, libraryPath string) (*FileScriptStore, error) {
	return &FileScriptStore{
		scripts: new(sync.Map),
		libs:    new(sync.Map),
	}, nil
}

func (f *FileScriptStore) GetScripts(ctx context.Context, subject string) (map[string][]byte, error) {
	res, ok := f.scripts.Load(subject)
	if !ok {
		return nil, fmt.Errorf("script not found for subject: %s", subject)
	}

	return res.(map[string][]byte), nil
}

func (f *FileScriptStore) AddScript(ctx context.Context, subject, name string, script []byte) error {
	sl, ok := f.scripts.Load(subject)
	if !ok {
		f.scripts.Store(subject, make(map[string][]byte))
		sl, _ = f.scripts.Load(subject)
	}

	scrm := sl.(map[string][]byte)
	scrm[name] = script
	f.scripts.Store(subject, scrm)

	return nil
}

func (f *FileScriptStore) DeleteScript(ctx context.Context, subject, name string) error {
	sl, ok := f.scripts.Load(subject)
	if !ok {
		return nil
	}

	scrm := sl.(map[string][]byte)
	delete(scrm, name)

	f.scripts.Store(subject, scrm)
	return nil
}

func (f *FileScriptStore) ReleaseLock(ctx context.Context, path string) error {
	return nil
}

func (f *FileScriptStore) TakeLock(ctx context.Context, path string) (bool, error) {
	return true, nil
}

// WatchScripts uses fsnotify to monitor the script file for changes
func (f *FileScriptStore) WatchScripts(ctx context.Context, subject string, onChange func(subject, path string, script []byte, deleted bool)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// Add the file to the watcher
	err = watcher.Add(f.filePath)
	if err != nil {
		log.Fatalf("failed to add file to watcher: %v", err)
	}

	log.Infof("Started watching file: %s", f.filePath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// We only care about write events, which means the file has been modified
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Infof("File modified: %s", f.filePath)

				scr, err := msgscript.ReadFile(filepath.Join(f.filePath, event.Name))
				if err != nil {
					log.Errorf("failed to read script file %s: %v", f.filePath, err)
				}

				// Trigger the onChange callback for each script
				onChange(scr.Subject, f.filePath, scr.Content, false)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Errorf("Watcher error: %v", err)

		case <-ctx.Done():
			log.Info("Stopping watcher")
			return
		}
	}
}

func (f *FileScriptStore) ListSubjects(ctx context.Context) ([]string, error) {
	var subjects []string

	f.scripts.Range(func(key, value interface{}) bool {
		subject := key.(string)
		subjects = append(subjects, subject)

		return true
	})

	return subjects, nil
}

func (f *FileScriptStore) LoadLibrairies(ctx context.Context, libraryPaths []string) ([][]byte, error) {
	var libraries [][]byte
	for _, path := range libraryPaths {
		v, ok := f.libs.Load(path)
		if ok {
			libraries = append(libraries, v.([]byte))
		}

		libraries = append(libraries, []byte(fmt.Sprintf("library %s", path)))
	}

	return libraries, nil
}

func (f *FileScriptStore) AddLibrary(ctx context.Context, content []byte, path string) error {
	f.libs.Store(path, []byte(content))
	return nil
}

func (f *FileScriptStore) RemoveLibrary(ctx context.Context, path string) error {
	f.libs.Delete(path)
	return nil
}
