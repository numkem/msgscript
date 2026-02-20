package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/numkem/msgscript/executor"
	"github.com/numkem/msgscript/store"
)

const (
	subjectListScripts        = "__listScrips"
	subjectListNamesForScript = "__listNamesForScript"
	subjectInfoNamedSCript    = "__infoNamedScript"
)

func replyWithSubjectList(ctx context.Context, nc *nats.Conn, scriptStore store.ScriptStore, replySubject string) {
	subjects, err := scriptStore.ListSubjects(ctx)
	if err != nil {
		replyWithError(nc, err, replySubject)
		return
	}

	j, err := json.Marshal(subjects)
	if err != nil {
		replyWithError(nc, fmt.Errorf("failed to encode subject list: %v", err), replySubject)
		return
	}

	replyMessage(nc, &executor.Message{}, replySubject, &Reply{
		Results: []*executor.ScriptResult{
			{
				Code:    http.StatusOK,
				Error:   "",
				Headers: map[string]string{},
				IsHTML:  true,
				Payload: j,
			},
		},
		HTML:  true,
		Error: "",
	})
}

func replyWithNamesForSubject(ctx context.Context, nc *nats.Conn, scriptStore store.ScriptStore, subject string, replySubject string) {
	scripts, err := scriptStore.GetScripts(ctx, subject)
	if err != nil {
		replyWithError(nc, err, replySubject)
		return
	}

	var names []string
	for name := range scripts {
		names = append(names, name)
	}

	j, err := json.Marshal(names)
	if err != nil {
		replyWithError(nc, fmt.Errorf("failed to encode name list: %v", err), replySubject)
		return
	}

	replyMessage(nc, &executor.Message{}, replySubject, &Reply{
		Results: []*executor.ScriptResult{
			{
				Code:    http.StatusOK,
				Error:   "",
				Headers: map[string]string{},
				IsHTML:  true,
				Payload: j,
			},
		},
		HTML:  true,
		Error: "",
	})
}

func replyWithNamedScriptInfo(ctx context.Context, nc *nats.Conn, scriptStore store.ScriptStore, subject, name, replySubject string) {
	allScripts, err := scriptStore.GetScripts(ctx, subject)
	if err != nil {
		replyWithError(nc, err, replySubject)
		return
	}

	script := allScripts[name]
	if script == nil {
		replyWithError(nc, fmt.Errorf("no script found for subject %s with name %s", subject, name), replySubject)
		return
	}

	if script.LibKeys == nil {
		script.LibKeys = []string{}
	}
	if script.Executor == "" {
		script.Executor = executor.EXECUTOR_LUA_NAME
	}

	replyMessage(nc, &executor.Message{}, replySubject, &Reply{
		Results: []*executor.ScriptResult{
			{
				Code:  http.StatusOK,
				Error: "",
				Headers: map[string]string{
					"libraries": strings.Join(script.LibKeys, ", "),
					"executor":  script.Executor,
				},
				IsHTML:  script.HTML,
				Payload: script.Content,
			},
		},
		HTML:  script.HTML,
		Error: "",
	})

}
