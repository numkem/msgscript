package store

import "context"

type DevStore struct {
	// Key pattern:
	// First: subject
	// Second: name
	// Value: content
	scripts map[string]map[string]string
}

func NewDevStore() ScriptStore {
	return &DevStore{
		scripts: make(map[string]map[string]string),
	}
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
