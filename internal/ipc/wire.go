// Package ipc is the niri-ipc 26.4.0 wire: one JSON value per line, with
// serde's externally tagged enums. A unit variant is a string ("Version"); a
// variant with fields is a one-key object ({"Action":{"Quit":{...}}}).
package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ErrParse is Niri's reply to a request it cannot decode. Niri sends only this
// context, not serde's detail (src/ipc/server.rs).
var ErrParse = errors.New("error parsing request")

// Request is a decoded request. Action is set when Name is "Action".
type Request struct {
	Name   string
	Action *Action
}

// Action is a decoded action. Only the fields of the actions built so far
// are kept; every other action is known by Name alone.
type Action struct {
	Name             string
	SkipConfirmation bool     // Quit
	Command          []string // Spawn
	ShellCommand     string   // SpawnSh
}

// DecodeRequest decodes one request line.
func DecodeRequest(line []byte) (Request, error) {
	name, body, err := variant(line)
	if err != nil || !slices.Contains(Requests, name) {
		return Request{}, ErrParse
	}
	r := Request{Name: name}
	if name == "Action" {
		a, err := decodeAction(body)
		if err != nil {
			return Request{}, ErrParse
		}
		r.Action = &a
	}
	return r, nil
}

func decodeAction(raw json.RawMessage) (Action, error) {
	name, body, err := variant(raw)
	if err != nil || !slices.Contains(Actions, name) {
		return Action{}, ErrParse
	}
	a := Action{Name: name}
	switch name {
	case "Quit":
		var f struct {
			SkipConfirmation *bool `json:"skip_confirmation"`
		}
		if err := strict(body, &f); err != nil || f.SkipConfirmation == nil {
			return Action{}, ErrParse
		}
		a.SkipConfirmation = *f.SkipConfirmation
	case "Spawn":
		var f struct {
			Command *[]string `json:"command"`
		}
		if err := strict(body, &f); err != nil || f.Command == nil {
			return Action{}, ErrParse
		}
		a.Command = *f.Command
	case "SpawnSh":
		var f struct {
			Command *string `json:"command"`
		}
		if err := strict(body, &f); err != nil || f.Command == nil {
			return Action{}, ErrParse
		}
		a.ShellCommand = *f.Command
	}
	return a, nil
}

// variant splits an externally tagged enum value into its name and body. A
// unit variant has no body.
func variant(raw []byte) (string, json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return name, nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) != 1 {
		return "", nil, ErrParse
	}
	for k, v := range obj {
		return k, v, nil
	}
	return "", nil, ErrParse
}

func strict(body json.RawMessage, v any) error {
	if body == nil {
		return ErrParse
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// EncodeRequest is the line a client sends, without its newline.
func EncodeRequest(r Request) []byte {
	if r.Name != "Action" || r.Action == nil {
		return marshal(r.Name)
	}
	a := r.Action
	var body any = struct{}{}
	switch a.Name {
	case "Quit":
		body = struct {
			SkipConfirmation bool `json:"skip_confirmation"`
		}{a.SkipConfirmation}
	case "Spawn":
		command := a.Command
		if command == nil {
			command = []string{}
		}
		body = struct {
			Command []string `json:"command"`
		}{command}
	case "SpawnSh":
		body = struct {
			Command string `json:"command"`
		}{a.ShellCommand}
	}
	return marshal(map[string]any{"Action": map[string]any{a.Name: body}})
}

// marshal writes JSON as serde_json does: '<', '>' and '&' are not escaped.
func marshal(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// Reply is Result<Response, String>: exactly one of Ok and Err is meaningful.
type Reply struct {
	Ok  json.RawMessage
	Err string
}

// Handled is the response to an action.
var Handled = json.RawMessage(`"Handled"`)

// VersionResponse is the response to Version.
func VersionResponse(v string) json.RawMessage {
	return marshal(map[string]string{"Version": v})
}

// Unimplemented is the error for a request or action that decodes but is not
// built yet (design §5).
func Unimplemented(name string) Reply { return Reply{Err: "unimplemented: " + name} }

// MarshalJSON writes {"Ok":…} or {"Err":"…"}.
func (r Reply) MarshalJSON() ([]byte, error) {
	if r.Ok == nil {
		return marshal(map[string]string{"Err": r.Err}), nil
	}
	return marshal(map[string]json.RawMessage{"Ok": r.Ok}), nil
}

// UnmarshalJSON reads {"Ok":…} or {"Err":"…"}.
func (r *Reply) UnmarshalJSON(b []byte) error {
	name, body, err := variant(b)
	if err != nil || body == nil {
		return fmt.Errorf("ipc: reply is not Ok or Err: %s", b)
	}
	switch name {
	case "Ok":
		r.Ok, r.Err = body, ""
		return nil
	case "Err":
		r.Ok = nil
		return json.Unmarshal(body, &r.Err)
	}
	return fmt.Errorf("ipc: reply is not Ok or Err: %s", b)
}
