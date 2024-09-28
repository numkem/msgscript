package script

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"
)

type LuamdaEndpoints struct {
	GET     []string
	POST    []string
	PUT     []string
	PATCH   []string
	DELETE  []string
	HEAD    []string
	OPTIONS []string
}

type LuamdaReader struct {
	Endpoints *LuamdaEndpoints
}

var LUAMDA_HTTP_VERBS = [7]string{
	http.MethodDelete,
	http.MethodGet,
	http.MethodHead,
	http.MethodOptions,
	http.MethodPatch,
	http.MethodPost,
	http.MethodPut,
}

func (l *LuamdaReader) ReadFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %s: %v", filename, err)
	}

	err = l.Read(f)
	if err != nil {
		return fmt.Errorf("failed to read file: %s: %v", filename, err)
	}

	return nil
}

func (l *LuamdaReader) ReadString(str string) error {
	r := strings.NewReader(str)

	err := l.Read(r)
	if err != nil {
		return fmt.Errorf("failed to read string buffer: %v", err)
	}

	return nil
}

func (l *LuamdaReader) Read(f io.Reader) error {
	l.Endpoints = new(LuamdaEndpoints)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, verb := range LUAMDA_HTTP_VERBS {
			if v := getHeaderValue(line, fmt.Sprintf("--* %s: ", verb)); v != "" {
				_, err := url.Parse(v)
				if err != nil {
					log.WithField("verb", "GET").WithField("url", v).Warnf("failed to find a valid URL")
					continue
				}

				// Following is a dynamic version of doing something like:
				// l.Endpoints.GET = append(l.Endpoints.GET, v)
				s := reflect.ValueOf(l.Endpoints)
				s = s.Elem()
				field := s.FieldByName(verb)
				ns := reflect.Append(field, reflect.ValueOf(v))
				field.Set(ns)
			}
		}
	}

	return nil
}
