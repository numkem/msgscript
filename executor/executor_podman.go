package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	spec "github.com/opencontainers/runtime-spec/specs-go"

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
	}, nil
}

func (pe *PodmanExecutor) HandleMessage(ctx context.Context, msg *Message, replyFunc func(*Reply)) {
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
	for path, ctnCfg := range ctnCfgs {
		wg.Add(1)

		ss := strings.Split(path, "/")
		name := ss[len(ss)-1]
		fields["path"] = name

		go func(content []byte) {
			defer wg.Done()

			result, err := pe.executeInContainer(ctx, msg)
			if err != nil {
				r.Error = err.Error()
			} else {
				r.Results.Store(msg.Subject, result)
			}
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

func (pe *PodmanExecutor) executeInContainer(ctx context.Context, msg *Message) (*ScriptResult, error) {
	// Get the configuration of the container from the message itself
	cfg := new(containerConfiguration)
	err := json.Unmarshal(msg.Payload, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode container configuration: %w", err)
	}

	if cfg.User == "" {
		cfg.User = "root"
	}
	if len(cfg.Groups) == 0 {
		cfg.Groups = []string{"root"}
	}

	containerName := "msgscript-" + uuid.New().String()[:8]

	f, err := createTempFile("msgscript-ctn-log-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	// Pull the requested image
	_, err = images.Pull(pe.ConnText, cfg.Image, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", err)
	}

	spec := &specgen.SpecGenerator{
		ContainerBasicConfig: specgen.ContainerBasicConfig{
			Name: containerName,
			Env:  map[string]string{"SUBJECT": msg.Subject, "URL": msg.URL, "PAYLOAD": string(msg.Payload), "METHOD": msg.Method},
			LogConfiguration: &specgen.LogConfig{
				Driver: "json-file",
				Path:   f.Name(),
			},
			Command: cfg.Command,
		},
		ContainerStorageConfig: specgen.ContainerStorageConfig{
			Image:  cfg.Image,
			Mounts: cfg.Mounts,
		},
		ContainerSecurityConfig: specgen.ContainerSecurityConfig{
			Privileged: &cfg.Privileged,
			User:       cfg.User,
			Groups:     cfg.Groups,
		},
	}

	container, err := containers.CreateWithSpec(pe.ConnText, spec, nil)
	if err != nil {
		return nil, err
	}

	err = containers.Start(pe.ConnText, container.ID, nil)
	if err != nil {
		return nil, err
	}
	pe.containers.Store(containerName, container.ID)

	exitCode, err := containers.Wait(pe.ConnText, container.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("Container exited with error: %w", err)
	}
	log.WithField("containerName", containerName).Info("container exited with code: %d", exitCode)

	pe.containers.Delete(containerName)

	// Read it's log file and return it's content
	payload, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	result := &ScriptResult{
		Code:    int(exitCode),
		Headers: make(map[string]string),
		Payload: payload,
	}

	return result, nil
}

func (pe *PodmanExecutor) Stop() {
	// Go through each running container and kill them
	pe.containers.Range(func(key, value interface{}) bool {
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
