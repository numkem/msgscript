//go:build podman

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/google/uuid"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	log "github.com/sirupsen/logrus"

	"github.com/numkem/msgscript/script"
	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

type PodmanExecutor struct {
	containers sync.Map
	store      msgstore.ScriptStore
	ConnText   context.Context
}

type containerConfiguration struct {
	Image      string       `json:"image"`
	Mounts     []spec.Mount `json:"mounts"`
	Command    []string     `json:"command"`
	Privileged bool         `json:"privileged"`
	User       string       `json:"user"`
	Groups     []string     `json:"groups"`
}

func NewPodmanExecutor(ctx context.Context, store msgstore.ScriptStore) (*PodmanExecutor, error) {
	// Get Podman socket location
	sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	socket := "unix:" + sock_dir + "/podman/podman.sock"

	// Connect to Podman socket
	connText, err := bindings.NewConnection(ctx, socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the podman socket: %w", err)
	}

	return &PodmanExecutor{
		ConnText: connText,
		store:    store,
	}, nil
}

func (pe *PodmanExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc ReplyFunc) {
	fields := log.Fields{
		"subject":  msg.Subject,
		"executor": "podman",
	}

	// Get the configuration from the store
	ctnCfgs, err := pe.store.GetScripts(ctx, msg.Subject)
	if err != nil || ctnCfgs == nil {
		replyWithError(fields, replyFunc, "failed to get scripts for subject %s: %w", msg.Subject, err)
		return
	}

	errs := make(chan error, len(ctnCfgs))
	var wg sync.WaitGroup
	r := NewReply()
	// Loop through each container configuration and execute the specified container
	for _, ctnCfg := range ctnCfgs {
		wg.Add(1)

		fields["name"] = ctnCfg.Name

		go func(ctnCfg *script.Script) {
			defer wg.Done()

			scr, err := scriptLib.ReadString(string(ctnCfg.Content))
			if err != nil {
				errs <- fmt.Errorf("failed to read script: %w", err)
				return
			}

			result, err := pe.executeInContainer(fields, scr, msg)
			if err != nil {
				errs <- fmt.Errorf("failed to execute container: %w", err)
			}

			r.Results.Store(msg.Subject, result)
		}(ctnCfg)
	}

	wg.Wait()
	log.WithField("subject", msg.Subject).Debugf("finished running %d containers", len(ctnCfgs))

	close(errs)
	for e := range errs {
		r.Error = r.Error + " " + e.Error()
	}

	replyFunc(r)
}

func (pe *PodmanExecutor) executeInContainer(fields log.Fields, scr *scriptLib.Script, msg *Message) (*ScriptResult, error) {
	// Get the configuration of the container from the message itself
	cfg := new(containerConfiguration)
	err := json.Unmarshal(scr.Content, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode container configuration: %w", err)
	}

	containerName := "msgscript-" + uuid.New().String()[:8]

	// Pull the requested image
	_, err = images.Pull(pe.ConnText, cfg.Image, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", cfg.Image, err)
	}

	spec := specgen.NewSpecGenerator(cfg.Image, false)
	spec.Command = cfg.Command
	spec.Name = containerName
	spec.Env = map[string]string{"SUBJECT": msg.Subject, "URL": msg.URL, "PAYLOAD": string(msg.Payload), "METHOD": msg.Method}
	spec.Mounts = cfg.Mounts
	spec.User = cfg.User
	spec.Groups = cfg.Groups
	spec.Privileged = &cfg.Privileged
	spec.Stdin = boolPtr(true)
	spec.Terminal = boolPtr(true)
	spec.Remove = boolPtr(true)

	fields["ctnName"] = containerName

	log.WithFields(fields).Debugf("creating container from spec")
	container, err := containers.CreateWithSpec(pe.ConnText, spec, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container with spec: %w", err)
	}
	fields["ctnID"] = container.ID

	stdout, err := createTempFile("msgscript-ctn-stdout-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer stdout.Close()
	defer os.Remove(stdout.Name())
	log.WithFields(fields).WithField("filename", stdout.Name()).Debugf("created stdout file")

	stderr, err := createTempFile("msgscript-ctn-stderr-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer stderr.Close()
	defer os.Remove(stderr.Name())
	log.WithFields(fields).WithField("filename", stderr.Name()).Debugf("created stderr file")

	stdin, stdinW := io.Pipe()
	go func() {
		defer stdinW.Close()
		_, err := stdinW.Write(msg.Payload)
		if err != nil {
			log.WithFields(fields).WithError(err).Error("failed to write to container stdin")
		}
	}()

	attachReady := make(chan bool)
	go func() {
		err := containers.Attach(pe.ConnText, container.ID, stdin, stdout, stderr, attachReady, nil)
		if err != nil {
			log.WithFields(fields).Errorf("failed to attach to container ID %s: %v", container.ID, err)
		}
	}()

	<-attachReady
	log.WithFields(fields).Debug("attached to container")
	pe.containers.Store(containerName, container.ID)

	log.WithFields(fields).Debug("starting container")
	err = containers.Start(pe.ConnText, container.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}
	log.WithFields(fields).Debug("started container")

	exitCode, err := containers.Wait(pe.ConnText, container.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("Container exited with error: %w", err)
	}
	log.WithField("containerName", containerName).Infof("container exited with code: %d", exitCode)

	pe.containers.Delete(containerName)
	log.WithFields(fields).Debug("container removed")

	// Read it's log file and return it's content
	resStdout, err := os.ReadFile(stdout.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read stdout file %s: %w", stdout.Name(), err)
	}

	resStderr, err := os.ReadFile(stderr.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read stderr file %s: %w", stderr.Name(), err)
	}

	log.WithFields(fields).Debugf("stdout: \n%+v", string(resStdout))
	log.WithFields(fields).Debugf("stderr: \n%+v", string(resStderr))

	result := &ScriptResult{
		Code:    int(exitCode),
		Headers: make(map[string]string),
		Payload: resStdout,
		Error:   string(resStderr),
	}

	return result, nil
}

func (pe *PodmanExecutor) Stop() {
	// Go through each running container and kill them
	pe.containers.Range(func(key, value any) bool {
		containers.Kill(pe.ConnText, value.(string), &containers.KillOptions{
			Signal: stringPtr("SIGKILL"),
		})
		return true
	})
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
