package main

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/numkem/msgscript/executor"
)

type Reply struct {
	Results []*executor.ScriptResult `json:"script_result"`
	HTML    bool                     `json:"is_html"`
	Error   string                   `json:"error,omitempty"`
}

func replyMessage(nc *nats.Conn, msg *executor.Message, replySubject string, rep *Reply) error {
	fields := log.Fields{
		"Subject": msg.Subject,
		"URL":     msg.URL,
		"Method":  msg.Method,
	}

	// Send a reply if the message has a reply subject
	if replySubject == "" {
		return nil
	}

	var payload []byte
	var err error
	payload, err = json.Marshal(rep)
	if err != nil {
		log.WithFields(fields).Errorf("failed to serialize script reply to JSON: %v", err)
		return fmt.Errorf("failed to serialize script reply to JSON: %v", err)
	}

	log.WithFields(fields).Debugf("sent reply: %s", string(payload))
	err = nc.Publish(replySubject, payload)
	if err != nil {
		log.WithFields(fields).Errorf("failed to publish reply after running script: %v", err)
	}

	return nil
}

func replyWithError(nc *nats.Conn, resErr error, replySubject string) {
	payload, err := json.Marshal(&Reply{Error: resErr.Error()})
	if err != nil {
		log.Errorf("failed to serialize script reply to JSON: %v", err)
	}

	err = nc.Publish(replySubject, payload)
	if err != nil {
		log.Errorf("failed to reply with error: %v", err)
	}
}
