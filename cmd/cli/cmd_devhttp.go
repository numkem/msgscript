package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	msgplugin "github.com/numkem/msgscript/plugins"
	"github.com/numkem/msgscript/script"
	"github.com/numkem/msgscript/store"
)

const DEVHTTP_SERVER_PORT = 7634

var devHttpCmd = &cobra.Command{
	Use:   "devhttp",
	Args:  validateArgIsPath,
	Short: "Starts a webserver that will run only to receive request from this script",
	Run:   devHttpCmdRun,
}

func init() {
	rootCmd.AddCommand(devHttpCmd)

	devHttpCmd.PersistentFlags().StringP("library", "l", "", "Path to a folder containing libraries to load for the function")
	devHttpCmd.PersistentFlags().StringP("pluginDir", "p", "", "Path to a folder with plugins")
}

func devHttpCmdRun(cmd *cobra.Command, args []string) {
	store, err := store.NewDevStore(cmd.Flag("library").Value.String())
	if err != nil {
		cmd.PrintErrf("failed to create store: %v\n", err)
		return
	}

	var plugins []msgplugin.PreloadFunc
	if path := cmd.Flag("pluginDir").Value.String(); path != "" {
		plugins, err = msgplugin.ReadPluginDir(path)
		if err != nil {
			cmd.PrintErrf("failed to read plugins: %v\n", err)
			return
		}
	}

	scriptExecutor := script.NewScriptExecutor(store, plugins, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fullpath, err := filepath.Abs(args[0])
	if err != nil {
		cmd.PrintErrf("failed to get absolute path for file %s: %v", args[0], err)
		return
	}

	go func() {
		proxy := &devHttpProxy{
			store:      store,
			executor:   scriptExecutor,
			context:    ctx,
			scriptFile: fullpath,
			libraryDir: cmd.Flag("library").Value.String(),
		}

		log.Infof("Starting HTTP server on port %d", DEVHTTP_SERVER_PORT)
		cmd.PrintErrln(http.ListenAndServe(fmt.Sprintf(":%d", DEVHTTP_SERVER_PORT), proxy))
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	cancel()

	cmd.Println("Received shutdown signal, stopping server...")
	scriptExecutor.Stop()
}

type devHttpProxy struct {
	store      store.ScriptStore
	executor   *script.ScriptExecutor
	context    context.Context
	scriptFile string
	libraryDir string
}

func (p *devHttpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// URL should look like /funcs.foobar
	// Where funcs.foobar is the subject for NATS
	ss := strings.Split(r.URL.Path, "/")
	// Validate URL structure
	if len(ss) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("URL should be in the pattern of /<subject>"))
		return
	}
	subject := ss[1]

	fields := log.Fields{
		"subject": subject,
		"client":  r.RemoteAddr,
		"method":  r.Method,
	}
	log.WithFields(fields).Info("Received HTTP request")

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("failed to read request body: %v", err)))
		return
	}

	// Load script from disk
	s := new(script.Script)
	err = s.ReadFile(p.scriptFile)
	if err != nil {
		log.WithField("filename", p.scriptFile).Errorf("failed to read file: %v", err)
		return
	}

	// TODO: load/delete libraries
	libs, err := parseDirsForLibraries([]string{p.libraryDir}, true)
	for _, lib := range libs {
		p.store.AddLibrary(r.Context(), string(lib.Content), lib.Name)
	}

	// Add only the currently worked on file
	p.store.AddScript(p.context, s.Subject, s.Name, string(s.Content))

	// Remove the script at the end of processing to be sure we load a fresh version every time
	defer emptyStore(p.store, p.libraryDir)

	url := strings.Replace(r.URL.String(), "/"+subject, "", -1)
	log.Infof("URL: %s", url)
	msg := &script.Message{
		Payload: payload,
		Method:  r.Method,
		Subject: subject,
		URL:     url,
	}

	p.executor.HandleMessage(p.context, msg, func(rep *script.Reply) {
		if rep.Error != "" {
			if rep.Error == (&script.NoScriptFoundError{}).Error() {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}

			_, err = w.Write([]byte("Error: " + rep.Error))
			if err != nil {
				log.Errorf("failed to write error to HTTP response: %v", err)
			}

			return
		}

		if rep.HTML != nil {
			w.Header().Add("Content-Type", "text/html")
			_, err = w.Write(rep.HTML)
			if err != nil {
				log.Errorf("failed to write reply back to HTTP response: %v", err)
			}
		} else {
			// Convert the results to bytes
			rr, err := json.Marshal(rep.AllResults)
			if err != nil {
				log.Errorf("failed to serialize all results to JSON: %v", err)
			}

			_, err = w.Write(rr)
			if err != nil {
				log.Errorf("failed to write reply back to HTTP response: %v", err)
			}
		}
	})
}

func emptyStore(s store.ScriptStore, libraryDir string) {
	s, _ = store.NewDevStore(libraryDir)
}
