//go:build !podman
// This is so that we can build without podman's dependancies

package executor

import (
	"context"

	msgstore "github.com/numkem/msgscript/store"
)

type noPodmanExecutor struct{}

func NewPodmanExecutor(c context.Context, store msgstore.ScriptStore) (Executor, error) {
	return &noPodmanExecutor{}, nil
}

func (e *noPodmanExecutor) HandleMessage(ctx context.Context, msg *Message, rf ReplyFunc) {
	r := NewReply()
	r.Error = "msgscript wasn't built with podman support"
	rf(r)
}

func (e *noPodmanExecutor) Stop() {}
