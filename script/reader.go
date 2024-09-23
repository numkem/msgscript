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
	Content []byte
	LibKeys []string
}

type ScriptReader struct {
	Script *Script
}

func (s *ScriptReader) ReadFile(filename string) error {
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

func (s *ScriptReader) ReadString(str string) error {
	r := strings.NewReader(str)

	err := s.Read(r)
	if err != nil {
		return fmt.Errorf("failed to read string buffer: %v", err)
	}

	return nil
}

func getHeaderValue(line, header string) string {
	if strings.HasPrefix(strings.ToLower(line), header) {
		return strings.TrimSpace(strings.Replace(line, header, "", 1))
	}

	return ""
}

func (s *ScriptReader) Read(f io.Reader) error {
	s.Script = new(Script)

	scanner := bufio.NewScanner(f)
	var b strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if v := getHeaderValue(line, "--* subject: "); v != "" {
			s.Script.Subject = v
		}
		if v := getHeaderValue(line, "--* name: "); v != "" {
			s.Script.Name = v
		}
		if v := getHeaderValue(line, "--* require: "); v != "" {
			s.Script.LibKeys = append(s.Script.LibKeys, v)
		}

		_, err := b.WriteString(line + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to builder: %v", err)
		}
	}

	s.Script.Content = []byte(strings.TrimSuffix(b.String(), "\n"))

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
