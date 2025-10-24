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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/numkem/msgscript/script"
	scriptLib "github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var podmanTracer = otel.Tracer("msgscript.executor.podman")

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

func (pe *PodmanExecutor) HandleMessage(ctx context.Context, msg *Message, scr *script.Script) *ScriptResult {
	ctx, span := podmanTracer.Start(ctx, "podman.handle_message",
		trace.WithAttributes(
			attribute.String("subject", msg.Subject),
			attribute.String("script.name", scr.Name),
			attribute.String("method", msg.Method),
			attribute.Int("payload_size", len(msg.Payload)),
		),
	)
	defer span.End()

	fields := log.Fields{
		"subject":  msg.Subject,
		"executor": "podman",
	}

	fields["name"] = scr.Name

	_, parseSpan := podmanTracer.Start(ctx, "podman.parse_script")
	scr, err := scriptLib.ReadString(string(scr.Content))
	if err != nil {
		parseSpan.RecordError(err)
		parseSpan.SetStatus(codes.Error, "Failed to parse script")
		parseSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse script")
		return ScriptResultWithError(fmt.Errorf("failed to read script: %w", err))
	}
	parseSpan.SetStatus(codes.Ok, "")
	parseSpan.End()

	res, err := pe.executeInContainer(ctx, fields, scr, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to execute container")
		return ScriptResultWithError(fmt.Errorf("failed to execute container: %w", err))
	}

	span.SetAttributes(
		attribute.Int("result.exit_code", res.Code),
		attribute.Int("result.payload_size", len(res.Payload)),
		attribute.Bool("result.has_error", res.Error != ""),
	)
	span.SetStatus(codes.Ok, "Container executed successfully")

	return res
}

func (pe *PodmanExecutor) executeInContainer(ctx context.Context, fields log.Fields, scr *scriptLib.Script, msg *Message) (*ScriptResult, error) {
	ctx, span := podmanTracer.Start(ctx, "podman.execute_container")
	defer span.End()

	// Parse container configuration
	_, cfgSpan := podmanTracer.Start(ctx, "podman.parse_config")
	cfg := new(containerConfiguration)
	err := json.Unmarshal(scr.Content, cfg)
	if err != nil {
		cfgSpan.RecordError(err)
		cfgSpan.SetStatus(codes.Error, "Failed to decode config")
		cfgSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to decode config")
		return nil, fmt.Errorf("failed to decode container configuration: %w", err)
	}
	cfgSpan.SetAttributes(
		attribute.String("container.image", cfg.Image),
		attribute.StringSlice("container.command", cfg.Command),
		attribute.Bool("container.privileged", cfg.Privileged),
		attribute.String("container.user", cfg.User),
		attribute.Int("container.mount_count", len(cfg.Mounts)),
	)
	cfgSpan.SetStatus(codes.Ok, "")
	cfgSpan.End()

	containerName := "msgscript-" + uuid.New().String()[:8]
	span.SetAttributes(attribute.String("container.name", containerName))

	// Pull the requested image
	_, pullSpan := podmanTracer.Start(ctx, "podman.pull_image",
		trace.WithAttributes(
			attribute.String("container.image", cfg.Image),
		),
	)
	_, err = images.Pull(pe.ConnText, cfg.Image, nil)
	if err != nil {
		pullSpan.RecordError(err)
		pullSpan.SetStatus(codes.Error, "Failed to pull image")
		pullSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to pull image")
		return nil, fmt.Errorf("failed to pull image %s: %w", cfg.Image, err)
	}
	pullSpan.SetStatus(codes.Ok, "")
	pullSpan.End()

	// Create container spec
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

	// Create container
	_, createSpan := podmanTracer.Start(ctx, "podman.create_container",
		trace.WithAttributes(
			attribute.String("container.name", containerName),
			attribute.String("container.image", cfg.Image),
		),
	)
	log.WithFields(fields).Debugf("creating container from spec")
	container, err := containers.CreateWithSpec(pe.ConnText, spec, nil)
	if err != nil {
		createSpan.RecordError(err)
		createSpan.SetStatus(codes.Error, "Failed to create container")
		createSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create container")
		return nil, fmt.Errorf("failed to create container with spec: %w", err)
	}
	fields["ctnID"] = container.ID
	createSpan.SetAttributes(attribute.String("container.id", container.ID))
	createSpan.SetStatus(codes.Ok, "")
	createSpan.End()

	span.SetAttributes(attribute.String("container.id", container.ID))

	// Create temp files
	_, stdoutFileSpan := podmanTracer.Start(ctx, "podman.create_stdout_file")
	stdout, err := createTempFile("msgscript-ctn-stdout-*")
	if err != nil {
		stdoutFileSpan.RecordError(err)
		stdoutFileSpan.SetStatus(codes.Error, "Failed to create stdout file")
		stdoutFileSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create stdout file")
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	stdoutFileSpan.SetAttributes(attribute.String("stdout_file", stdout.Name()))
	stdoutFileSpan.SetStatus(codes.Ok, "")
	stdoutFileSpan.End()
	defer stdout.Close()
	defer os.Remove(stdout.Name())
	log.WithFields(fields).WithField("filename", stdout.Name()).Debugf("created stdout file")

	_, stderrFileSpan := podmanTracer.Start(ctx, "podman.create_stderr_file")
	stderr, err := createTempFile("msgscript-ctn-stderr-*")
	if err != nil {
		stderrFileSpan.RecordError(err)
		stderrFileSpan.SetStatus(codes.Error, "Failed to create stderr file")
		stderrFileSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create stderr file")
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	stderrFileSpan.SetAttributes(attribute.String("stderr_file", stderr.Name()))
	stderrFileSpan.SetStatus(codes.Ok, "")
	stderrFileSpan.End()
	defer stderr.Close()
	defer os.Remove(stderr.Name())
	log.WithFields(fields).WithField("filename", stderr.Name()).Debugf("created stderr file")

	// Setup stdin pipe
	stdin, stdinW := io.Pipe()
	go func() {
		defer stdinW.Close()
		_, err := stdinW.Write(msg.Payload)
		if err != nil {
			log.WithFields(fields).WithError(err).Error("failed to write to container stdin")
		}
	}()

	// Attach to container
	_, attachSpan := podmanTracer.Start(ctx, "podman.attach_container",
		trace.WithAttributes(
			attribute.String("container.id", container.ID),
		),
	)
	attachReady := make(chan bool)
	go func() {
		err := containers.Attach(pe.ConnText, container.ID, stdin, stdout, stderr, attachReady, nil)
		if err != nil {
			log.WithFields(fields).Errorf("failed to attach to container ID %s: %v", container.ID, err)
		}
	}()

	<-attachReady
	log.WithFields(fields).Debug("attached to container")
	attachSpan.SetStatus(codes.Ok, "")
	attachSpan.End()

	pe.containers.Store(containerName, container.ID)

	// Start container
	_, startSpan := podmanTracer.Start(ctx, "podman.start_container",
		trace.WithAttributes(
			attribute.String("container.id", container.ID),
		),
	)
	log.WithFields(fields).Debug("starting container")
	err = containers.Start(pe.ConnText, container.ID, nil)
	if err != nil {
		startSpan.RecordError(err)
		startSpan.SetStatus(codes.Error, "Failed to start container")
		startSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start container")
		return nil, fmt.Errorf("failed to start container: %w", err)
	}
	log.WithFields(fields).Debug("started container")
	startSpan.SetStatus(codes.Ok, "")
	startSpan.End()

	// Wait for container to finish
	_, waitSpan := podmanTracer.Start(ctx, "podman.wait_container",
		trace.WithAttributes(
			attribute.String("container.id", container.ID),
		),
	)
	exitCode, err := containers.Wait(pe.ConnText, container.ID, nil)
	if err != nil {
		waitSpan.RecordError(err)
		waitSpan.SetStatus(codes.Error, "Container wait failed")
		waitSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Container wait failed")
		return nil, fmt.Errorf("Container exited with error: %w", err)
	}
	log.WithField("containerName", containerName).Infof("container exited with code: %d", exitCode)
	waitSpan.SetAttributes(attribute.Int("container.exit_code", int(exitCode)))
	if exitCode != 0 {
		waitSpan.SetStatus(codes.Error, fmt.Sprintf("Container exited with code %d", exitCode))
	} else {
		waitSpan.SetStatus(codes.Ok, "")
	}
	waitSpan.End()

	pe.containers.Delete(containerName)
	log.WithFields(fields).Debug("container removed")

	// Read stdout
	_, readStdoutSpan := podmanTracer.Start(ctx, "podman.read_stdout")
	resStdout, err := os.ReadFile(stdout.Name())
	if err != nil {
		readStdoutSpan.RecordError(err)
		readStdoutSpan.SetStatus(codes.Error, "Failed to read stdout")
		readStdoutSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read stdout")
		return nil, fmt.Errorf("failed to read stdout file %s: %w", stdout.Name(), err)
	}
	readStdoutSpan.SetAttributes(attribute.Int("stdout_size", len(resStdout)))
	readStdoutSpan.SetStatus(codes.Ok, "")
	readStdoutSpan.End()

	// Read stderr
	_, readStderrSpan := podmanTracer.Start(ctx, "podman.read_stderr")
	resStderr, err := os.ReadFile(stderr.Name())
	if err != nil {
		readStderrSpan.RecordError(err)
		readStderrSpan.SetStatus(codes.Error, "Failed to read stderr")
		readStderrSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read stderr")
		return nil, fmt.Errorf("failed to read stderr file %s: %w", stderr.Name(), err)
	}
	readStderrSpan.SetAttributes(
		attribute.Int("stderr_size", len(resStderr)),
		attribute.Bool("has_stderr", len(resStderr) > 0),
	)
	readStderrSpan.SetStatus(codes.Ok, "")
	readStderrSpan.End()

	log.WithFields(fields).Debugf("stdout: \n%+v", string(resStdout))
	log.WithFields(fields).Debugf("stderr: \n%+v", string(resStderr))

	result := &ScriptResult{
		Code:    int(exitCode),
		Headers: make(map[string]string),
		Payload: resStdout,
		Error:   string(resStderr),
	}

	span.SetAttributes(
		attribute.Int("result.exit_code", int(exitCode)),
		attribute.Int("result.stdout_size", len(resStdout)),
		attribute.Int("result.stderr_size", len(resStderr)),
	)

	if exitCode == 0 && len(resStderr) == 0 {
		span.SetStatus(codes.Ok, "Container executed successfully")
	} else {
		span.SetStatus(codes.Error, fmt.Sprintf("Container exited with code %d", exitCode))
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
