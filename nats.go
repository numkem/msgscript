package msgscript

import (
	"os"
)

func NatsUrlByEnv() string {
	return os.Getenv("NATS_URL")
}
