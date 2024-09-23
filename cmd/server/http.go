package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

const DEFAULT_HTTP_PORT = 7643

type httpNatsProxy struct {
	port string
	nc   *nats.Conn
}

func NewHttpNatsProxy(port int, natsURL string) (*httpNatsProxy, error) {
	// Connect to NATS
	if natsURL == "" {
		natsURL = natsUrlByEnv()
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to NATS: %v", err)
	}

	return &httpNatsProxy{
		nc: nc,
	}, nil
}

func (p *httpNatsProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// We only support POST requests
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Only POST request are supported"))
		return
	}
	defer r.Body.Close()

	// URL should look like /funcs.foobar/yo
	// Where funcs.foobar is the subject for NATS
	// Where yo would be the name of the script
	ss := strings.Split(r.URL.Path, "/")
	// Validate URL structure
	if len(ss) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("URL should be in the pattern of /<subject>"))
		return
	}
	subject := ss[1]

	fields := log.Fields{
		"subject": subject,
		"client":  r.RemoteAddr,
	}
	log.WithFields(fields).Info("Received HTTP request")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("failed to read request body: %v", err)))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second*5)
	defer cancel()

	msg, err := p.nc.RequestWithContext(ctx, subject, body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(msg.Data))
	}
}

func runHTTP(port int, natsURL string) {
	proxy, err := NewHttpNatsProxy(port, natsURL)
	if err != nil {
		log.Fatalf("failed to create HTTP proxy: %v", err)
	}

	log.Infof("Starting HTTP server on port %d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), proxy))
}
