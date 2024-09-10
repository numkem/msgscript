package store

// import (
// 	"context"
// 	"database/sql"
// 	"sync"
// )

// type sqliteScriptStore struct {
// 	db *sql.DB
// }

// func NewSqliteScriptStore(dbPath string) (*sqliteScriptStore, error) {
// 	db, err := sql.Open("sqlite3", dbPath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &sqliteScriptStore{db: db}, nil
// }

// func (s *sqliteScriptStore) LoadInitialScripts() (*sync.Map, error) {
// 	rows, err := s.db.Query("SELECT subject, script FROM scripts")
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	scripts := new(sync.Map)
// 	for rows.Next() {
// 		var subject, script string
// 		if err := rows.Scan(&subject, &script); err != nil {
// 			return nil, err
// 		}
// 		scripts.Store(subject, script)
// 	}
// 	return scripts, nil
// }

// func (s *sqliteScriptStore) WatchScripts(ctx context.Context, onChange func(subject, path, script string, deleted bool)) {
// 	// SQLite doesn't support native watches, so this can be no-op or use polling.
// }

// func (s *sqliteScriptStore) GetScript(subject string) (string, error) {
// 	var script string
// 	err := s.db.QueryRow("SELECT script FROM scripts WHERE subject = ?", subject).Scan(&script)
// 	if err != nil {
// 		return "", err
// 	}
// 	return script, nil
// }
