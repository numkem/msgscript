package store

import "context"

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
}
