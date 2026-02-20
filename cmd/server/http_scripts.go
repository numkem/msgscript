package main

import (
	"encoding/json"
	"fmt"
	"html"
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

	sort.Strings(subjects)

	w.WriteHeader(http.StatusOK)
	resp := []byte(`
<html>
  <header>
    <title>Msgscript :: List all subjects</title>
  </header>
  <body>
    <h1>Subject List</h1><ul>`)

	for _, subject := range subjects {
		if subject == "" {
			continue
		}

		resp = fmt.Appendf(resp, `<li><a href="/_/subject/%s">%s</a></li>`, subject, subject)
	}

	w.Write(fmt.Append(resp, `</ul></body></html>`))
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

	var names []string
	err = json.Unmarshal(rep.Results[0].Payload, &names)
	if err != nil {
		returnError(w, err)
		return
	}

	sort.Strings(names)

	w.WriteHeader(http.StatusOK)
	resp := fmt.Appendf(nil, `<html>
  <header>
    <title>Msgscript :: List all subjects</title>
  </header>
  <body>
    <h1>Names for subject %s</h1><ul>`, subject)

	for _, name := range names {
		if name == "" {
			continue
		}

		resp = fmt.Appendf(resp, `<li><a href="/_/info/%s/%s">%s</a></li>`, subject, name, name)
	}

	w.Write(fmt.Append(resp, `</ul></body></html>`))
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

	script := rep.Results[0]

	isHTMLValue := "No"
	if script.IsHTML {
		isHTMLValue = fmt.Sprintf(`<a href="/%s/">Yes</a>`, subject)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(fmt.Appendf(nil, `<html>
  <header>
    <title>Msgscript :: Info for script %s/%s</title>
  </header>
  <body>
    <h1>Script information</h1>
    <h2>Properties</h2>
    Subject: %s<br />
    Name: %s<br />
    Is HTML?: %s<br />
    Libraires used: %s<br />
    Executor: %s<br />

    <h2>Source</h2>
    <pre>
%s
    </pre>
  </body>
</html>
`, subject, name, subject, name, isHTMLValue, script.Headers["libraries"], script.Headers["executor"], html.EscapeString(string(script.Payload))))
}
