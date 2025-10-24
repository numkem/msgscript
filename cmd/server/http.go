package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/numkem/msgscript"
	"github.com/numkem/msgscript/executor"
)

const DEFAULT_HTTP_PORT = 7643
const DEFAULT_HTTP_TIMEOUT = 5 * time.Second

var tracer = otel.Tracer("http-nats-proxy")

type httpNatsProxy struct {
	port string
	nc   *nats.Conn
}

func NewHttpNatsProxy(port int, natsURL string) (*httpNatsProxy, error) {
	// Connect to NATS
	if natsURL == "" {
		natsURL = msgscript.NatsUrlByEnv()
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to NATS: %w", err)
	}

	return &httpNatsProxy{
		nc: nc,
	}, nil
}

func (p *httpNatsProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract context from incoming request headers
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	// Start a new span for the HTTP request
	ctx, span := tracer.Start(ctx, "http.request",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.remote_addr", r.RemoteAddr),
		),
	)
	defer span.End()

	defer r.Body.Close()

	// URL should look like /funcs.foobar
	// Where funcs.foobar is the subject for NATS
	ss := strings.Split(r.URL.Path, "/")
	// Validate URL structure
	if len(ss) < 2 {
		span.SetStatus(codes.Error, "Invalid URL structure")
		span.SetAttributes(attribute.Int("http.status_code", http.StatusBadRequest))
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("URL should be in the pattern of /<subject>"))
		return
	}
	subject := ss[1]
	span.SetAttributes(attribute.String("nats.subject", subject))

	fields := log.Fields{
		"subject": subject,
		"client":  r.RemoteAddr,
	}
	log.WithFields(fields).Info("Received HTTP request")

	// Read request body with tracing
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read request body")
		span.SetAttributes(attribute.Int("http.status_code", http.StatusInternalServerError))
		w.WriteHeader(http.StatusInternalServerError)
		_, err = fmt.Fprintf(w, "failed to read request body: %s", err)
		if err != nil {
			log.WithFields(fields).Errorf("failed to write payload: %v", err)
		}
		return
	}
	span.SetAttributes(attribute.Int("http.request.body_size", len(payload)))

	// We can override the HTTP timeout by passing the `_timeout` query string
	timeout := DEFAULT_HTTP_TIMEOUT
	if r.URL.Query().Has("_timeout") {
		timeout, err = time.ParseDuration(r.URL.Query().Get("_timeout"))
		if err != nil {
			timeout = DEFAULT_HTTP_TIMEOUT
		}
	}
	span.SetAttributes(attribute.String("http.timeout", timeout.String()))

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Change the url passed to the fuction to remove the subject
	url := strings.ReplaceAll(r.URL.String(), "/"+subject, "")
	log.Debugf("URL: %s", url)
	body, err := json.Marshal(&executor.Message{
		Payload: payload,
		Method:  r.Method,
		Subject: subject,
		URL:     url,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to encode message")
		log.WithFields(fields).Errorf("failed to encode message: %v", err)
		return
	}

	// Start a child span for the NATS request
	ctx, natsSpan := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("nats.subject", subject),
			attribute.Int("nats.message_size", len(body)),
		),
	)

	// Inject trace context into NATS message headers
	msg := nats.NewMsg(subject)
	msg.Data = body
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))

	// Send the message and wait for the response
	response, err := p.nc.RequestMsgWithContext(ctx, msg)
	if err != nil {
		natsSpan.RecordError(err)
		natsSpan.SetStatus(codes.Error, "NATS request failed")
		natsSpan.End()

		span.SetStatus(codes.Error, "Service unavailable")
		span.SetAttributes(attribute.Int("http.status_code", http.StatusServiceUnavailable))

		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}
	natsSpan.SetAttributes(attribute.Int("nats.response_size", len(msg.Data)))
	natsSpan.SetStatus(codes.Ok, "")
	natsSpan.End()

	rep := new(Reply)
	err = json.Unmarshal(response.Data, rep)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal response")
		span.SetAttributes(attribute.Int("http.status_code", http.StatusFailedDependency))
		w.WriteHeader(http.StatusFailedDependency)
		fmt.Fprintf(w, "Error: %v", err)
		return
	}

	if rep.Error != "" {
		span.SetAttributes(attribute.String("executor.error", rep.Error))
		if rep.Error == (&executor.NoScriptFoundError{}).Error() {
			span.SetStatus(codes.Error, "Script not found")
			span.SetAttributes(attribute.Int("http.status_code", http.StatusNotFound))
			w.WriteHeader(http.StatusNotFound)
		} else {
			span.SetStatus(codes.Error, rep.Error)
			span.SetAttributes(attribute.Int("http.status_code", http.StatusInternalServerError))
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, err = w.Write([]byte("Error: " + rep.Error))
		if err != nil {
			log.WithFields(fields).Errorf("failed to write error to HTTP response: %v", err)
		}

		return
	}

	// Go through all the scripts to see if one is HTML
	for _, scrRes := range rep.Results {
		if scrRes.IsHTML {

			span.SetAttributes(attribute.Bool("response.is_html", true))
			var hasContentType bool
			for k, v := range scrRes.Headers {
				if k == "Content-Type" {
					hasContentType = true
				}
				w.Header().Add(k, v)
			}
			if !hasContentType {
				w.Header().Add("Content-Type", "text/html")
			}
			span.SetAttributes(
				attribute.Int("http.status_code", scrRes.Code),
				attribute.Int("http.response.body_size", len(scrRes.Payload)),
			)
			w.WriteHeader(scrRes.Code)

			_, err = w.Write(scrRes.Payload)
			if err != nil {
				span.RecordError(err)
				log.WithFields(fields).Errorf("failed to write reply back to HTTP response: %v", err)
			}

			span.SetStatus(codes.Ok, "")
			// Since only the HTML page reply can "win" we ignore the rest
			return
		}
	}

	// Convert the results to bytes
	span.SetAttributes(attribute.Bool("response.is_html", false))
	rr, err := json.Marshal(rep.Results)
	if err != nil {
		span.RecordError(err)
		log.WithFields(fields).Errorf("failed to serialize all results to JSON: %v", err)
	}

	span.SetAttributes(
		attribute.Int("http.status_code", http.StatusOK),
		attribute.Int("http.response.body_size", len(rr)),
	)

	_, err = w.Write(rr)
	if err != nil {
		span.RecordError(err)
		log.WithFields(fields).Errorf("failed to write reply back to HTTP response: %v", err)
	}

	span.SetStatus(codes.Ok, "")
}

func hasHTMLResult(results map[string]*executor.ScriptResult) (bool, *executor.ScriptResult) {
	for _, sr := range results {
		if sr.IsHTML {
			return true, sr
		}
	}

	return false, nil
}

func runHTTP(port int, natsURL string) {
	proxy, err := NewHttpNatsProxy(port, natsURL)
	if err != nil {
		log.Fatalf("failed to create HTTP proxy: %v", err)
	}

	log.Infof("Starting HTTP server on port %d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), proxy))
}
