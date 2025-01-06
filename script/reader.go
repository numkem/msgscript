package script

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Script struct {
	Name    string
	Subject string
	HTML    bool
	Content []byte
	LibKeys []string
}

func (s *Script) ReadFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filename, err)
	}

	err = s.Read(f)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filename, err)
	}

	return nil
}

func (s *Script) ReadString(str string) error {
	r := strings.NewReader(str)
	return s.Read(r)
}

func getHeaderValue(line, header string) string {
	if strings.HasPrefix(line, header) {
		return strings.TrimSpace(strings.Replace(line, header, "", 1))
	}

	return ""
}

func (s *Script) Read(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	var b strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if v := getHeaderValue(line, "--* subject: "); v != "" {
			s.Subject = v
		}
		if v := getHeaderValue(line, "--* name: "); v != "" {
			s.Name = v
		}
		if v := getHeaderValue(line, "--* require: "); v != "" {
			s.LibKeys = append(s.LibKeys, v)
		}
		if v := getHeaderValue(line, "--* html:"); v != "" {
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
