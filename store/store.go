package store

// Available backend options
const (
	BACKEND_ETCD_NAME   = "etcd"
	BACKEND_SQLITE_NAME = "sqlite"
	BACKEND_FILE_NAME   = "file"
)

type ScriptStore interface {
	WatchScripts(subject string, onChange func(subject, path, script string, deleted bool))
	GetScripts(subject string) ([]string, error)
	AddScript(subject string, name string, script string) error
	DeleteScript(subject, scriptID string) error
}
