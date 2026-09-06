// Package bridge implements an ephemeral, metadata-only stdio control channel.
package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

const Version = 0
const MaxRequestBytes = 65536
const MaxSnapshotBytes = 2 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var sessionPattern = regexp.MustCompile(`^\$[0-9]{1,10}$`)
var windowPattern = regexp.MustCompile(`^@[0-9]{1,10}$`)
var panePattern = regexp.MustCompile(`^%[0-9]{1,10}$`)
var generationPattern = regexp.MustCompile(`^server:[0-9a-f]{64}$`)

type Request struct {
	Version   int             `json:"version"`
	RequestID string          `json:"requestId"`
	Command   string          `json:"command"`
	Params    json.RawMessage `json:"params"`
}
type Params struct {
	ExpectedGeneration string `json:"expectedGeneration,omitempty"`
	OperationID        string `json:"operationId,omitempty"`
	SessionID          string `json:"sessionId,omitempty"`
	WindowID           string `json:"windowId,omitempty"`
	PaneID             string `json:"paneId,omitempty"`
	AttachmentID       string `json:"attachmentId,omitempty"`
	Name               string `json:"name,omitempty"`
	Cwd                string `json:"cwd,omitempty"`
	Direction          string `json:"direction,omitempty"`
	Confirm            bool   `json:"confirm,omitempty"`
}
type Response struct {
	Version   int        `json:"version"`
	Type      string     `json:"type"`
	RequestID string     `json:"requestId"`
	OK        bool       `json:"ok"`
	Result    any        `json:"result,omitempty"`
	Error     *WireError `json:"error,omitempty"`
}
type WireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Snapshot struct {
	BridgeID    string            `json:"bridgeId"`
	Sequence    uint64            `json:"sequence"`
	ObservedAt  string            `json:"observedAt"`
	Server      tmux.BridgeServer `json:"server"`
	Sessions    []Session         `json:"sessions"`
	Attachments []Attachment      `json:"attachments"`
}
type Session struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Attached bool     `json:"attached"`
	Windows  []Window `json:"windows"`
}
type Window struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	Layout    string `json:"layout"`
	Panes     []Pane `json:"panes"`
}
type Pane struct {
	ID             string `json:"id"`
	WindowID       string `json:"windowId"`
	Index          int    `json:"index"`
	Active         bool   `json:"active"`
	Cwd            string `json:"cwd"`
	CurrentCommand string `json:"currentCommand"`
}
type Attachment struct {
	AttachmentID string  `json:"attachmentId"`
	State        string  `json:"state"`
	SessionID    *string `json:"sessionId"`
	WindowID     *string `json:"windowId"`
	PaneID       *string `json:"paneId"`
}
type Result struct {
	Generation   string `json:"generation"`
	SessionID    string `json:"sessionId,omitempty"`
	WindowID     string `json:"windowId,omitempty"`
	PaneID       string `json:"paneId,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
	Ticket       string `json:"ticket,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

func textOK(s string, max int, empty bool) bool {
	if len(s) > max || (!empty && s == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func strict(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return errors.New("invalid-utf8")
	}
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing-json")
	}
	return nil
}
func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate-json-field")
				}
				seen[name] = true
				if e = walk(); e != nil {
					return e
				}
			}
			_, err = d.Token()
			return err
		case json.Delim('['):
			for d.More() {
				if e := walk(); e != nil {
					return e
				}
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing-json")
	}
	return nil
}
func decode(raw []byte) (Request, Params, error) {
	var r Request
	var p Params
	top := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &top) != nil || len(top) != 4 || (top["version"] == nil || bytes.Equal(bytes.TrimSpace(top["version"]), []byte("null"))) || top["requestId"] == nil || top["command"] == nil || top["params"] == nil {
		return r, p, errors.New("INVALID_REQUEST")
	}
	if len(raw) > MaxRequestBytes || strict(raw, &r) != nil || !idPattern.MatchString(r.RequestID) || len(r.Params) == 0 {
		return r, p, errors.New("INVALID_REQUEST")
	}
	if r.Version != Version {
		return r, p, errors.New("UNSUPPORTED_VERSION")
	}
	fields := map[string]json.RawMessage{}
	if len(r.Params) == 0 || r.Params[0] != '{' || strict(r.Params, &fields) != nil || strict(r.Params, &p) != nil {
		return r, p, errors.New("INVALID_REQUEST")
	}
	common := []string{"expectedGeneration", "operationId"}
	var required, optional []string
	switch r.Command {
	case "snapshot":
		required = []string{}
	case "attach.status":
		required = []string{"attachmentId"}
	case "session.create":
		required = append(common, "name", "cwd")
	case "session.select":
		required = append(common, "sessionId", "attachmentId")
	case "session.rename":
		required = append(common, "sessionId", "name")
	case "session.kill":
		required = append(common, "sessionId", "confirm")
	case "window.create":
		required = append(common, "sessionId", "name", "cwd")
	case "window.select":
		required = append(common, "sessionId", "windowId", "attachmentId")
	case "window.rename":
		required = append(common, "sessionId", "windowId", "name")
	case "window.kill":
		required = append(common, "sessionId", "windowId", "confirm")
	case "pane.select":
		required = append(common, "sessionId", "windowId", "paneId", "attachmentId")
	case "pane.split":
		required = append(common, "sessionId", "windowId", "paneId", "direction", "cwd")
	case "attach.issue":
		required = append(common, "sessionId")
		optional = []string{"windowId"}
	default:
		return r, p, errors.New("UNKNOWN_COMMAND")
	}
	allowed := map[string]bool{}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return r, p, errors.New("INVALID_REQUEST")
		}
		allowed[key] = true
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return r, p, errors.New("INVALID_REQUEST")
		}
	}
	if _, ok := fields["expectedGeneration"]; ok && (!generationPattern.MatchString(p.ExpectedGeneration) || !idPattern.MatchString(p.OperationID)) {
		return r, p, errors.New("INVALID_REQUEST")
	}
	for field, rule := range map[string]*regexp.Regexp{"sessionId": sessionPattern, "windowId": windowPattern, "paneId": panePattern, "attachmentId": idPattern} {
		if value, ok := fields[field]; ok {
			var s string
			if json.Unmarshal(value, &s) != nil || !rule.MatchString(s) {
				return r, p, errors.New("INVALID_REQUEST")
			}
		}
	}
	if value, ok := fields["name"]; ok {
		var name string
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &name) != nil || !textOK(name, 128, r.Command == "session.create" || r.Command == "window.create") {
			return r, p, errors.New("INVALID_REQUEST")
		}
	}
	if _, ok := fields["cwd"]; ok && !textOK(p.Cwd, 4096, false) {
		return r, p, errors.New("INVALID_REQUEST")
	}
	if _, ok := fields["direction"]; ok && p.Direction != "horizontal" && p.Direction != "vertical" {
		return r, p, errors.New("INVALID_REQUEST")
	}
	if _, ok := fields["confirm"]; ok && !p.Confirm {
		return r, p, errors.New("CONFIRM_REQUIRED")
	}
	return r, p, nil
}
