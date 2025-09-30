package script

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	HEADER_PATTERN      = "--*"
	LIBRARY_FOLDER_NAME = "libs"
)

type Script struct {
	Content  []byte   `json:"content"`
	Executor string   `json:"executor"`
	HTML     bool     `json:"is_html"`
	LibKeys  []string `json:"libraries"`
	Name     string   `json:"name"`
	Subject  string   `json:"subject"`
}

func ReadFile(filename string) (*Script, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}

	s := new(Script)
	err = s.Read(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	return s, nil
}

func ReadString(content string) (*Script, error) {
	r := strings.NewReader(content)
	s := new(Script)

	err := s.Read(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read string content: %w", err)
	}

	return s, nil
}

func getHeaderKey(line string) string {
	if strings.HasPrefix(line, HEADER_PATTERN) {
		ss := strings.Split(line, " ")
		if len(ss) >= 2 {
			return strings.TrimSuffix(ss[1], ":")
		}
	}

	return ""
}

func getHeaderValue(line string) string {
	ss := strings.Split(line, " ")
	if len(ss) >= 3 {
		return strings.Join(ss[2:], " ")
	}

	return ""
}

func (s *Script) Read(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	var err error
	var b strings.Builder
	for scanner.Scan() {
		line := scanner.Text()

		k := getHeaderKey(line)
		v := getHeaderValue(line)
		switch k {
		case "subject":
			s.Subject = v
		case "name":
			s.Name = v
		case "require":
			s.LibKeys = append(s.LibKeys, v)
		case "html":
			s.HTML, err = strconv.ParseBool(v)
			if err != nil {
				s.HTML = false
			}
		case "executor":
			s.Executor = v
		default:
			_, err := b.WriteString(line + "\n")
			if err != nil {
				return fmt.Errorf("failed to write to builder: %w", err)
			}
		}
	}

	s.Content = []byte(strings.TrimSuffix(b.String(), "\n"))

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
					return fmt.Errorf("failed to read script %s: %w", fullname, err)
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
			return nil, fmt.Errorf("failed to walk directory %s: %w", dirname, err)
		}
	} else {
		entries, err := os.ReadDir(dirname)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, e := range entries {
			if path.Ext(e.Name()) == ".lua" {
				fullname := path.Join(dirname, e.Name())

				s, err := ReadFile(fullname)
				if err != nil {
					return nil, fmt.Errorf("failed to read script %s: %w", fullname, err)
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
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, e := range entries {
		if path.Ext(e.Name()) == ".lua" {
			fullname := path.Join(dirname, e.Name())

			s, err := ReadFile(fullname)
			if err != nil {
				return nil, fmt.Errorf("failed to read script %s: %w", fullname, err)
			}

			// Add the script to the map
			libraries = append(libraries, s)
		}
	}

	return libraries, nil
}
