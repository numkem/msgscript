package store

import (
	"context"
	"fmt"
)

// Available backend options
const (
	BACKEND_ETCD_NAME   = "etcd"
	BACKEND_SQLITE_NAME = "sqlite"
	BACKEND_FILE_NAME   = "file"
)

type ScriptStore interface {
	AddScript(ctx context.Context, subject string, name string, script string) error
	DeleteScript(ctx context.Context, subject, scriptID string) error
	GetScripts(ctx context.Context, subject string) (map[string]string, error)
	ReleaseLock(ctx context.Context, path string) error
	TakeLock(ctx context.Context, path string) (bool, error)
	WatchScripts(ctx context.Context, subject string, onChange func(subject, path, script string, deleted bool))
	ListSubjects(ctx context.Context) ([]string, error)
	LoadLibrairies(ctx context.Context, libraryPaths []string) ([]string, error)
	AddLibrary(ctx context.Context, library, path string) error
	RemoveLibrary(ctx context.Context, path string) error
}

func StoreByName(name, etcdEndpoints string) (ScriptStore, error) {
	switch name {
	case BACKEND_ETCD_NAME:
		scriptStore, err := NewEtcdScriptStore(etcdEndpoints)
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize etcd store: %v", err)
		}

		return scriptStore, nil
		// case BACKEND_SQLITE_NAME:
		// 	// Initialize SQLite backend (placeholder for now)
		// 	scriptStore, err = msgstore.NewSqliteScriptStore("path/to/db.sqlite") // implement this
		// 	if err != nil {
		// 		log.Fatalf("Failed to initialize SQLite store: %v", err)
		// 	}
		// case BACKEND_FILE_NAME:
		// 	// Initialize flat file backend (placeholder for now)
		// 	scriptStore, err = msgstore.NewFileScriptStore("path/to/scripts") // implement this
		// 	if err != nil {
		//
		// 		log.Fatalf("Failed to initialize file store: %v", err)
		// 	}
	}

	return nil, fmt.Errorf("Unknown backend: %s", name)
}
