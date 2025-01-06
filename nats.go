package msgscript

import (
	"os"

	"github.com/nats-io/nats.go"
)

func NatsUrlByEnv() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	} else {
		return nats.DefaultURL
	}
}
