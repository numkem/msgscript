package store

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

func (s *DevStore) WatchScripts(subject string, onChange func(subject, name, script string, delete bool)) {
	onChange(subject, "", "", false)
}

func (s *DevStore) AddScript(subject, name, script string) error {
	s.onChange(subject, name, script, false)

	return nil
}

func (s *DevStore) DeleteScript(subject, name string) error {
	s.onChange(subject, name, "", true)

	return nil
}

func (s *DevStore) GetScripts(subject string) ([]string, error) {
	var scriptsForSubject []string

	for _, s := range s.scripts[subject] {
		scriptsForSubject = append(scriptsForSubject, s)
	}

	return scriptsForSubject, nil
}
