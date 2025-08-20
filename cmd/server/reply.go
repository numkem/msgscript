package main

import (
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/numkem/msgscript/executor"
)

type replier struct {
	nc *nats.Conn
}

func (rep *replier) SyncReply(m *executor.Message, msg *nats.Msg) executor.ReplyFunc {
	return func(r *executor.Reply) {
		fields := log.Fields{
			"Subject": m.Subject,
			"URL":     m.URL,
			"Method":  m.Method,
		}

		if r.Error != "" {
			log.WithFields(fields).Errorf("error while running script: %s", r.Error)
		}

		// Send a reply if the message has a reply subject
		if msg.Reply == "" {
			return
		}

		var reply []byte
		var err error
		if !m.Raw {
			reply, err = r.JSON()
			if err != nil {
				log.WithFields(fields).Errorf("failed to serialize script reply to JSON: %v", err)
			}
		} else {
			if r.Error != "" {
				reply = []byte(r.Error)
			} else {
				reply = r.Bytes()
			}
		}

		log.WithFields(fields).Debugf("sent reply: %s", reply)
		err = rep.nc.Publish(msg.Reply, reply)
		if err != nil {
			log.WithFields(fields).Errorf("failed to publish reply after running script: %v", err)
		}
	}
}

func (re *replier) AsyncReply(m *executor.Message, msg *nats.Msg) executor.ReplyFunc {
	return func(r *executor.Reply) {
		fields := log.Fields{
			"Subject": m.Subject,
			"URL":     m.URL,
			"Method":  m.Method,
		}

		if r.Error != "" {
			log.WithFields(fields).Errorf("error while running script: %s", r.Error)
		}

		log.WithFields(fields).Debug("async reply received")
	}
}
