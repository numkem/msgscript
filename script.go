package msgscript

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const (
	HEADER_PATTERN      = "--*"
	LIBRARY_FOLDER_NAME = "libs"
)

type Script struct {
	Name    string
	Subject string
	HTML    bool
	Content []byte
	LibKeys []string
}

func ReadFile(filename string) (*Script, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", filename, err)
	}

	s := new(Script)
	err = s.Read(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", filename, err)
	}

	return s, nil
}

func ReadString(content string) (*Script, error) {
	r := strings.NewReader(content)
	s := new(Script)

	err := s.Read(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read string content: %v", err)
	}

	return s, nil
}

func getHeaderValue(line, header string) string {
	if strings.HasPrefix(line, header) {
		return strings.TrimSpace(strings.Replace(line, header, "", 1))
	}

	return ""
}

func headerKey(key string) string {
	return fmt.Sprintf("%s %s: ", HEADER_PATTERN, key)
}

func (s *Script) Read(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	var b strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if v := getHeaderValue(line, headerKey("subject")); v != "" {
			s.Subject = v
		}
		if v := getHeaderValue(line, headerKey("name")); v != "" {
			s.Name = v
		}
		if v := getHeaderValue(line, headerKey("require")); v != "" {
			s.LibKeys = append(s.LibKeys, v)
		}
		if v := getHeaderValue(line, headerKey("html")); v != "" {
			s.HTML = true
		}

		_, err := b.WriteString(line + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to builder: %v", err)
		}
	}

	s.Content = []byte(strings.TrimSuffix(b.String(), "\n"))

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func ReadScriptDirectory(dirname string, recurse bool) (map[string]map[string]*Script, error) {
	scripts := make(map[string]map[string]*Script)
	if recurse {
		fsys := os.DirFS(dirname)
		err := fs.WalkDir(fsys, ".", func(filename string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if path.Ext(filename) == ".lua" {
				fullname := path.Join(dirname, filename)

				// if the script is contained into a folder for libraries, ignore it
				// TODO: might have to revise this
				if strings.Contains(filename, fmt.Sprintf("%s/", LIBRARY_FOLDER_NAME)) {
					return nil
				}

				s, err := ReadFile(fullname)
				if err != nil {
					return fmt.Errorf("failed to read script %s: %v", fullname, err)
				}

				if s.Subject == "" {
					return fmt.Errorf("script required to have a 'subject' header")
				}
				if s.Name == "" {
					return fmt.Errorf("script required to have a 'name' header")
				}

				// Add the script to the map
				if _, e := scripts[s.Subject]; !e {
					scripts[s.Subject] = make(map[string]*Script)
				}

				scripts[s.Subject][s.Name] = s
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory %s: %v", dirname, err)
		}
	} else {
		entries, err := os.ReadDir(dirname)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %v", err)
		}

		for _, e := range entries {
			if path.Ext(e.Name()) == ".lua" {
				fullname := path.Join(dirname, e.Name())

				s, err := ReadFile(fullname)
				if err != nil {
					return nil, fmt.Errorf("failed to read script %s: %v", fullname, err)
				}

				// Add the script to the map
				if _, e := scripts[s.Subject]; !e {
					scripts[s.Subject] = make(map[string]*Script)
				}

				scripts[s.Subject][s.Name] = s
			}
		}
	}

	return scripts, nil
}

func ReadLibraryDirectory(dirname string) ([]*Script, error) {
	var libraries []*Script
	entries, err := os.ReadDir(dirname)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %v", err)
	}

	for _, e := range entries {
		if path.Ext(e.Name()) == ".lua" {
			fullname := path.Join(dirname, e.Name())

			s, err := ReadFile(fullname)
			if err != nil {
				return nil, fmt.Errorf("failed to read script %s: %v", fullname, err)
			}

			// Add the script to the map
			libraries = append(libraries, s)
		}
	}

	return libraries, nil
}
