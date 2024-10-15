package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/numkem/msgscript/script"
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
	defer r.Body.Close()

	// URL should look like /funcs.foobar
	// Where funcs.foobar is the subject for NATS
	ss := strings.Split(r.URL.Path, "/")
	// Validate URL structure
	if len(ss) < 2 {
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

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("failed to read request body: %v", err)))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second*5)
	defer cancel()

	url := strings.Replace(r.URL.String(), "/"+subject, "", -1)
	log.Debug(url)
	body, err := json.Marshal(&script.Message{
		Payload: payload,
		Method:  r.Method,
		Subject: subject,
		URL:     url,
	})
	if err != nil {
		log.Errorf("failed to encode message: %v", err)
		return
	}

	msg, err := p.nc.RequestWithContext(ctx, subject, body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	rep := new(script.Reply)
	err = json.Unmarshal(msg.Data, rep)
	if err != nil {
		w.WriteHeader(http.StatusFailedDependency)
		w.Write([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	if rep.Error != "" {
		if rep.Error == (&script.NoScriptFoundError{}).Error() {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, err = w.Write([]byte("Error: " + rep.Error))
		if err != nil {
			log.Errorf("failed to write error to HTTP response: %v", err)
		}

		return
	}

	if rep.HTML != nil {
		w.Header().Add("Content-Type", "text/html")
		_, err = w.Write(rep.HTML)
		if err != nil {
			log.Errorf("failed to write reply back to HTTP response: %v", err)
		}
	} else {
		// Convert the results to bytes
		rr, err := json.Marshal(rep.AllResults)
		if err != nil {
			log.Errorf("failed to serialize all results to JSON: %v", err)
		}

		_, err = w.Write(rr)
		if err != nil {
			log.Errorf("failed to write reply back to HTTP response: %v", err)
		}
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
