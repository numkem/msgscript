package store

import (
	"context"
	"fmt"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/numkem/msgscript/script"
)

// Available backend options
const (
	BACKEND_ETCD_NAME   = "etcd"
	BACKEND_SQLITE_NAME = "sqlite"
	BACKEND_FILE_NAME   = "file"
)

type ScriptStore interface {
	AddScript(ctx context.Context, subject string, name string, script []byte) error
	DeleteScript(ctx context.Context, subject, name string) error
	GetScripts(ctx context.Context, subject string) (map[string][]byte, error)
	ReleaseLock(ctx context.Context, path string) error
	TakeLock(ctx context.Context, path string) (bool, error)
	WatchScripts(ctx context.Context, subject string, onChange func(subject, path string, script []byte, deleted bool))
	ListSubjects(ctx context.Context) ([]string, error)
	LoadLibrairies(ctx context.Context, libraryPaths []string) ([][]byte, error)
	AddLibrary(ctx context.Context, content []byte, path string) error
	RemoveLibrary(ctx context.Context, path string) error
}

func StoreByName(name, etcdEndpoints, scriptDir, libraryDir string) (ScriptStore, error) {
	switch name {
	case BACKEND_ETCD_NAME:
		scriptStore, err := NewEtcdScriptStore(etcdEndpoints)
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize etcd store: %v", err)
		}

		return scriptStore, nil

	case BACKEND_FILE_NAME:
		// Validate that the script directory isn't empty at the very least
		if scriptDir == "" {
			return nil, fmt.Errorf("script directory cannot be empty")
		}
		// Convert it to an absolute, easier for debugging
		scriptDir, err := filepath.Abs(scriptDir)
		if err != nil {
			return nil, fmt.Errorf("failed to convert script directory %s to an aboslute", scriptDir)
		}

		scriptStore, err := NewFileScriptStore(scriptDir, libraryDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize file store: %v", err)
		}

		// Read all the scripts from the scripts directory and add them to the store
		allScripts, err := script.ReadScriptDirectory(scriptDir, false)
		if err != nil {
			return nil, fmt.Errorf("failed to read scripts: %v", err)
		}

		var nbScripts int
		for subject, namedScripts := range allScripts {
			for name, scr := range namedScripts {
				log.WithField("subject", subject).WithField("name", name).Debug("loading script")
				scriptStore.AddScript(context.Background(), subject, name, scr.Content)
				nbScripts++
			}
		}
		log.Infof("loaded %d scripts from %s", nbScripts, scriptDir)

		if libraryDir != "" {
			// Read libraries from the library directory
			allLibrairies, err := script.ReadLibraryDirectory(libraryDir)
			if err != nil {
				return nil, fmt.Errorf("failed to read library directory: %v", err)
			}

			var nbLibraries int
			for _, library := range allLibrairies {
				scriptStore.AddLibrary(context.Background(), library.Content, library.Name)
				nbLibraries++
			}
			log.Infof("loaded %d libraries from %s", nbLibraries, libraryDir)
		}

		return scriptStore, nil
	}

	return nil, fmt.Errorf("Unknown backend: %s", name)
}
