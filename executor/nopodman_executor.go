//go:build !podman

// This is so that we can build without podman's dependancies

package executor

import (
	"context"
	"fmt"

	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

type noPodmanExecutor struct{}

func NewPodmanExecutor(c context.Context, store msgstore.ScriptStore) (Executor, error) {
	return &noPodmanExecutor{}, nil
}

func (e *noPodmanExecutor) HandleMessage(ctx context.Context, msg *Message, scr *script.Script) *ScriptResult {
	return ScriptResultWithError(fmt.Errorf("msgscript wasn't built with podman support"))
}

func (e *noPodmanExecutor) Stop() {}
