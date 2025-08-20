package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/yuin/gopher-lua"
)

type PreloadFunc func(L *lua.LState, envs map[string]string)

func ReadPluginDir(dirpath string) ([]PreloadFunc, error) {
	entries, err := os.ReadDir(dirpath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory %s: %w", dirpath, err)
	}

	var readPlugins []PreloadFunc
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".so" {
			continue
		}

		fullPath := filepath.Join(dirpath, entry.Name())
		p, err := plugin.Open(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open plugin file %s: %w", fullPath, err)
		}

		symPreload, err := p.Lookup("Preload")
		if err != nil {
			return nil, fmt.Errorf("failed to find Plugin symbol: %w", err)
		}

		mp, ok := symPreload.(func(*lua.LState, map[string]string))
		if !ok {
			return nil, fmt.Errorf("invalid plugin: %s", fullPath)
		}

		log.WithField("plugin", fullPath).Debug("loaded plugin")

		readPlugins = append(readPlugins, mp)
	}

	return readPlugins, nil
}

func LoadPlugins(L *lua.LState, plugins []PreloadFunc) error {
	envs := make(map[string]string)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		envs[pair[0]] = pair[1]
	}

	for _, preload := range plugins {
		preload(L, envs)
	}

	return nil
}

func LoadPluginsFromDir(L *lua.LState, dirpath string) error {
	plugins, err := ReadPluginDir(dirpath)
	if err != nil {
		return err
	}

	LoadPlugins(L, plugins)

	return nil
}
