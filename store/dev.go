package store

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
)

type DevStore struct {
	// Key pattern:
	// First: subject
	// Second: name
	// Value: content
	scripts   map[string]map[string]string
	libraries map[string]string
}

func NewDevStore(libraryPath string) (ScriptStore, error) {
	store := &DevStore{
		scripts:   make(map[string]map[string]string),
		libraries: make(map[string]string),
	}

	if libraryPath != "" {
		stat, err := os.Stat(libraryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read library path %s: %v", libraryPath, err)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("given library path %s isn't a directory", libraryPath)
		}

		fsys := os.DirFS(libraryPath)
		err = fs.WalkDir(fsys, ".", func(filename string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if path.Ext(filename) == ".lua" {
				filepath := path.Join(libraryPath, filename)
				content, err := os.ReadFile(filepath)
				if err != nil {
					return fmt.Errorf("failed to read %s: %v", filename, err)
				}

				p := strings.Replace(strings.Replace(filename, libraryPath, "", 1), path.Ext(filename), "", 1)
				log.Debugf("loading library %s", p)
				store.AddLibrary(context.Background(), string(content), p)
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk through directory %s for library files: %v", libraryPath, err)
		}
	}

	return store, nil
}

func (s *DevStore) onChange(subject, name, script string, del bool) {
	if !del {
		if _, found := s.scripts[subject]; !found {
			s.scripts[subject] = make(map[string]string)
		}

		s.scripts[subject][name] = script
	} else {
		delete(s.scripts[subject], name)
	}

	return
}

func (s *DevStore) WatchScripts(ctx context.Context, subject string, onChange func(subject, name, script string, delete bool)) {
	onChange(subject, "", "", false)
}

func (s *DevStore) AddScript(ctx context.Context, subject, name, script string) error {
	s.onChange(subject, name, script, false)

	return nil
}

func (s *DevStore) DeleteScript(ctx context.Context, subject, name string) error {
	s.onChange(subject, name, "", true)

	return nil
}

func (s *DevStore) GetScripts(ctx context.Context, subject string) (map[string]string, error) {
	return s.scripts[subject], nil
}

func (s *DevStore) TakeLock(ctx context.Context, path string) (bool, error) {
	return true, nil
}

func (s *DevStore) ReleaseLock(ctx context.Context, path string) error {
	return nil
}

func (s *DevStore) ListSubjects(ctx context.Context) ([]string, error) {
	var subjects []string

	for subject := range s.scripts {
		subjects = append(subjects, subject)
	}

	return subjects, nil
}

func (s *DevStore) LoadLibrairies(ctx context.Context, libraryPaths []string) ([]string, error) {
	var libraries []string
	for _, path := range libraryPaths {
		if l, found := s.libraries[path]; found {
			libraries = append(libraries, l)
		}
	}

	return libraries, nil
}

func (s *DevStore) AddLibrary(ctx context.Context, content string, path string) error {
	s.libraries[path] = content

	return nil
}

func (s *DevStore) RemoveLibrary(ctx context.Context, path string) error {
	delete(s.libraries, path)

	return nil
}
