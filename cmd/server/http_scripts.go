package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
)

func (fh *functionHandler) ListScripts(w http.ResponseWriter, r *http.Request) {
	msg := nats.NewMsg(subjectListScripts)
	msg.Data = []byte("")

	// Send the message and wait for the response
	response, err := fh.nc.RequestMsgWithContext(r.Context(), msg)
	if err != nil {
		returnError(w, err)
		return
	}

	rep := &Reply{}
	err = json.Unmarshal(response.Data, rep)
	if err != nil {
		returnError(w, err)
		return
	}

	var subjects []string
	err = json.Unmarshal(rep.Results[0].Payload, &subjects)
	if err != nil {
		returnError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fh.templates["list"].ExecuteTemplate(w, "list", map[string]any{
		"subjects": subjects,
	})
}

func (fh *functionHandler) ListNamesForScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	subject := vars["subject"]

	msg := nats.NewMsg(subjectListNamesForScript)
	msg.Data = []byte(subject)

	// Send the message and wait for the response
	response, err := fh.nc.RequestMsgWithContext(r.Context(), msg)
	if err != nil {
		returnError(w, err)
		return
	}

	rep := &Reply{}
	err = json.Unmarshal(response.Data, rep)
	if err != nil {
		returnError(w, err)
		return
	}

	if len(rep.Results) < 1 {
		returnError(w, fmt.Errorf("no named script found for subject %s", subject))
		return
	}

	var names []string
	err = json.Unmarshal(rep.Results[0].Payload, &names)
	if err != nil {
		returnError(w, err)
		return
	}

	sort.Strings(names)

	w.WriteHeader(http.StatusOK)
	err = fh.templates["names"].ExecuteTemplate(w, "names", map[string]any{
		"subject": subject,
		"names":   names,
	})
	if err != nil {
		returnError(w, err)
		return
	}
}

func (fh *functionHandler) InfoForNamedScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	subject := vars["subject"]
	name := vars["name"]

	msg := nats.NewMsg(subjectInfoNamedSCript)
	msg.Data = []byte(strings.Join([]string{subject, name}, ","))

	response, err := fh.nc.RequestMsgWithContext(r.Context(), msg)
	if err != nil {
		returnError(w, err)
		return
	}

	rep := &Reply{}
	err = json.Unmarshal(response.Data, rep)
	if err != nil {
		returnError(w, err)
		return
	}

	if len(rep.Results) < 1 {
		returnError(w, fmt.Errorf("no information found on script for subject %s with name %s", subject, name))
		return
	}
	script := rep.Results[0]

	w.WriteHeader(http.StatusOK)
	err = fh.templates["info"].ExecuteTemplate(w, "info", map[string]any{
		"subject":     subject,
		"name":        name,
		"isHTMLValue": script.IsHTML,
		"libraries":   script.Headers["libraries"],
		"executor":    script.Headers["executor"],
		"content":     string(script.Payload),
	})
	if err != nil {
		returnError(w, err)
		return
	}
}
