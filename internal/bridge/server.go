package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

type operation struct {
	hash     string
	response Response
	at       time.Time
}
type Server struct {
	backend    tmux.BridgeBackend
	tickets    *ticketStore
	sequence   uint64
	operations map[string]operation
	lang       i18n.Lang
}

func New(backend tmux.BridgeBackend, lang i18n.Lang) (*Server, error) {
	tickets, err := newTicketStore()
	if err != nil {
		return nil, err
	}
	return &Server{backend: backend, tickets: tickets, operations: map[string]operation{}, lang: lang}, nil
}
func (s *Server) Close() { s.tickets.close(); clear(s.operations) }
func (s *Server) observe(ctx context.Context) (Snapshot, []tmux.BridgeClient, error) {
	for attempt := 0; attempt < 2; attempt++ {
		server, err := s.backend.BridgeServer(ctx)
		if err != nil {
			return Snapshot{}, nil, err
		}
		tree := []tmux.Session{}
		clients := []tmux.BridgeClient{}
		if server.Running {
			tree, err = s.backend.BridgeTree(ctx)
			if err != nil {
				continue
			}
			clients, err = s.backend.BridgeClients(ctx)
			if err != nil {
				continue
			}
		}
		after, err := s.backend.BridgeServer(ctx)
		if err != nil || after.Generation != server.Generation {
			continue
		}
		snapshot := Snapshot{BridgeID: s.tickets.id, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Server: server, Sessions: []Session{}}
		windows, panes := 0, 0
		if len(tree) > 256 {
			return Snapshot{}, nil, errors.New("LIMIT_EXCEEDED")
		}
		for _, native := range tree {
			if !sessionPattern.MatchString(native.ID) || !textOK(native.Name, 512, false) {
				return Snapshot{}, nil, errors.New("METADATA_UNAVAILABLE")
			}
			session := Session{ID: native.ID, Name: native.Name, Attached: native.Attached, Windows: []Window{}}
			for _, nw := range native.Windows {
				windows++
				if !windowPattern.MatchString(nw.ID) || nw.SessionID != native.ID || nw.Index < 0 || !textOK(nw.Name, 512, true) || !textOK(nw.Layout, 65536, true) {
					return Snapshot{}, nil, errors.New("METADATA_UNAVAILABLE")
				}
				window := Window{ID: nw.ID, SessionID: native.ID, Index: nw.Index, Name: nw.Name, Active: nw.Active, Layout: nw.Layout, Panes: []Pane{}}
				for _, np := range nw.Panes {
					panes++
					if !panePattern.MatchString(np.ID) || np.WindowID != nw.ID || np.Index < 0 || !textOK(np.CurrentPath, 4096, true) || !textOK(np.CurrentCommand, 256, true) {
						return Snapshot{}, nil, errors.New("METADATA_UNAVAILABLE")
					}
					window.Panes = append(window.Panes, Pane{np.ID, np.WindowID, np.Index, np.Active, np.CurrentPath, np.CurrentCommand})
				}
				session.Windows = append(session.Windows, window)
			}
			snapshot.Sessions = append(snapshot.Sessions, session)
		}
		if windows > 1024 || panes > 4096 {
			return Snapshot{}, nil, errors.New("LIMIT_EXCEEDED")
		}
		snapshot.Attachments = s.tickets.states(server, clients)
		sort.Slice(snapshot.Attachments, func(i, j int) bool {
			return snapshot.Attachments[i].AttachmentID < snapshot.Attachments[j].AttachmentID
		})
		s.sequence++
		snapshot.Sequence = s.sequence
		encoded, _ := json.Marshal(snapshot)
		if len(encoded) > MaxSnapshotBytes-4096 {
			return Snapshot{}, nil, errors.New("LIMIT_EXCEEDED")
		}
		return snapshot, clients, nil
	}
	return Snapshot{}, nil, errors.New("SERVER_CHANGED")
}
func (s *Server) failure(id, code string) Response {
	return Response{Version: Version, Type: "response", RequestID: id, Error: &WireError{code, i18n.T(s.lang, i18n.BridgeOperationFailed, code)}}
}
func errorCode(err error) string {
	if errors.Is(err, tmux.ErrBridgeGeneration) {
		return "SERVER_CHANGED"
	}
	for _, code := range []string{"INVALID_REQUEST", "UNSUPPORTED_VERSION", "UNKNOWN_COMMAND", "NO_SERVER", "SERVER_CHANGED", "NOT_FOUND", "OWNERSHIP_MISMATCH", "CONFIRM_REQUIRED", "OPERATION_CONFLICT", "LIMIT_EXCEEDED", "TICKET_INVALID", "TICKET_EXPIRED", "ATTACHMENT_NOT_CONNECTED", "METADATA_UNAVAILABLE"} {
		if err.Error() == code {
			return code
		}
	}
	return "COMMAND_FAILED"
}
func (s *Server) Handle(ctx context.Context, raw []byte) Response {
	request, p, err := decode(raw)
	if err != nil {
		return s.failure(request.RequestID, errorCode(err))
	}
	snapshot, clients, err := s.observe(ctx)
	if err != nil {
		return s.failure(request.RequestID, errorCode(err))
	}
	if request.Command == "snapshot" {
		return Response{Version: Version, Type: "response", RequestID: request.RequestID, OK: true, Result: snapshot}
	}
	if request.Command == "attach.status" {
		for _, a := range snapshot.Attachments {
			if a.AttachmentID == p.AttachmentID {
				return Response{Version: Version, Type: "response", RequestID: request.RequestID, OK: true, Result: a}
			}
		}
		return s.failure(request.RequestID, "NOT_FOUND")
	}
	canonical, _ := json.Marshal(struct {
		Command string
		Params  Params
	}{request.Command, p})
	hash := sha256.Sum256(canonical)
	digest := hex.EncodeToString(hash[:])
	now := time.Now()
	for key, value := range s.operations {
		if now.Sub(value.at) > 5*time.Minute {
			delete(s.operations, key)
		}
	}
	if previous, ok := s.operations[p.OperationID]; ok {
		if previous.hash != digest {
			return s.failure(request.RequestID, "OPERATION_CONFLICT")
		}
		replayGeneration := p.ExpectedGeneration
		if value, ok := previous.response.Result.(Result); previous.response.OK && ok {
			replayGeneration = value.Generation
		}
		if snapshot.Server.Generation != replayGeneration {
			return s.failure(request.RequestID, "SERVER_CHANGED")
		}
		response := previous.response
		response.RequestID = request.RequestID
		return response
	}
	if snapshot.Server.Generation != p.ExpectedGeneration {
		return s.failure(request.RequestID, "SERVER_CHANGED")
	}
	var result any
	result, err = s.mutate(ctx, request.Command, p, snapshot, clients)
	response := Response{Version: Version, Type: "response", RequestID: request.RequestID, OK: true, Result: result}
	if err != nil {
		response = s.failure(request.RequestID, errorCode(err))
	}
	if len(s.operations) >= 256 {
		oldest := ""
		for key, value := range s.operations {
			if oldest == "" || value.at.Before(s.operations[oldest].at) {
				oldest = key
			}
		}
		delete(s.operations, oldest)
	}
	s.operations[p.OperationID] = operation{digest, response, now}
	return response
}
func belongs(snapshot Snapshot, p Params) error {
	if p.SessionID == "" {
		return nil
	}
	for _, session := range snapshot.Sessions {
		if session.ID != p.SessionID {
			continue
		}
		if p.WindowID == "" {
			return nil
		}
		for _, window := range session.Windows {
			if window.ID != p.WindowID {
				continue
			}
			if p.PaneID == "" {
				return nil
			}
			for _, pane := range window.Panes {
				if pane.ID == p.PaneID {
					return nil
				}
			}
		}
		return errors.New("OWNERSHIP_MISMATCH")
	}
	return errors.New("NOT_FOUND")
}
func (s *Server) mutate(ctx context.Context, command string, p Params, snapshot Snapshot, clients []tmux.BridgeClient) (any, error) {
	if command != "session.create" && !snapshot.Server.Running {
		return nil, errors.New("NO_SERVER")
	}
	if err := belongs(snapshot, p); err != nil {
		return nil, err
	}
	if p.Cwd != "" {
		if !filepath.IsAbs(p.Cwd) {
			return nil, errors.New("INVALID_REQUEST")
		}
		st, err := os.Stat(p.Cwd)
		if err != nil || !st.IsDir() {
			return nil, errors.New("INVALID_REQUEST")
		}
	}
	if command == "attach.issue" {
		return s.tickets.issue(snapshot.Server, p.SessionID, p.WindowID)
	}
	result := Result{Generation: snapshot.Server.Generation, SessionID: p.SessionID, WindowID: p.WindowID, PaneID: p.PaneID}
	var args []string
	targetWindow := p.SessionID + ":" + p.WindowID
	targetPane := targetWindow + "." + p.PaneID
	switch command {
	case "session.create":
		args = []string{"new-session", "-d", "-P", "-F", "#{session_id}"}
		if p.Name != "" {
			args = append(args, "-s", strings.ReplaceAll(p.Name, "#", "##"))
		}
		args = append(args, "-c", strings.ReplaceAll(p.Cwd, "#", "##"))
	case "session.rename":
		args = []string{"rename-session", "-t", p.SessionID, strings.ReplaceAll(p.Name, "#", "##")}
	case "session.kill":
		args = []string{"kill-session", "-t", p.SessionID}
	case "window.create":
		args = []string{"new-window", "-d", "-P", "-F", "#{window_id}", "-t", p.SessionID + ":", "-c", strings.ReplaceAll(p.Cwd, "#", "##")}
		if p.Name != "" {
			args = append(args, "-n", strings.ReplaceAll(p.Name, "#", "##"))
		}
	case "window.rename":
		args = []string{"rename-window", "-t", targetWindow, strings.ReplaceAll(p.Name, "#", "##")}
	case "window.kill":
		args = []string{"kill-window", "-t", targetWindow}
	case "pane.split":
		args = []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-t", targetPane, "-c", strings.ReplaceAll(p.Cwd, "#", "##")}
		if p.Direction == "horizontal" {
			args = append(args, "-h")
		} else {
			args = append(args, "-v")
		}
	case "session.select", "window.select", "pane.select":
		client, err := s.tickets.client(p.AttachmentID, snapshot.Server, clients)
		if err != nil {
			return nil, err
		}
		target := p.SessionID
		if p.WindowID != "" {
			target = targetWindow
		}
		if _, err = s.backend.BridgeMutate(ctx, snapshot.Server, []string{"switch-client", "-c", client.TTY, "-t", target}, ""); err != nil {
			return nil, err
		}
		if command == "pane.select" {
			args = []string{"select-pane", "-t", targetPane}
		} else {
			args = nil
		}
		result.AttachmentID = p.AttachmentID
	default:
		return nil, errors.New("UNKNOWN_COMMAND")
	}
	output := ""
	var err error
	if len(args) > 0 {
		output, err = s.backend.BridgeMutate(ctx, snapshot.Server, args, "")
		if err != nil {
			return nil, err
		}
	}
	switch command {
	case "session.create":
		result.SessionID = strings.TrimSpace(output)
		if !sessionPattern.MatchString(result.SessionID) {
			return nil, errors.New("METADATA_UNAVAILABLE")
		}
	case "window.create":
		result.WindowID = strings.TrimSpace(output)
		if !windowPattern.MatchString(result.WindowID) {
			return nil, errors.New("METADATA_UNAVAILABLE")
		}
	case "pane.split":
		result.PaneID = strings.TrimSpace(output)
		if !panePattern.MatchString(result.PaneID) {
			return nil, errors.New("METADATA_UNAVAILABLE")
		}
	}
	after, err := s.backend.BridgeServer(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot.Server.Running && after.Running && after.Generation != snapshot.Server.Generation {
		return nil, errors.New("SERVER_CHANGED")
	}
	if command == "session.kill" || command == "window.kill" {
		verified, _, err := s.observe(ctx)
		if err != nil {
			return nil, err
		}
		if verified.Server.Generation != after.Generation {
			return nil, errors.New("SERVER_CHANGED")
		}
		for _, session := range verified.Sessions {
			if command == "session.kill" && session.ID == p.SessionID {
				return nil, errors.New("METADATA_UNAVAILABLE")
			}
			for _, window := range session.Windows {
				if command == "window.kill" && window.ID == p.WindowID {
					return nil, errors.New("METADATA_UNAVAILABLE")
				}
			}
		}
	}
	result.Generation = after.Generation
	return result, nil
}

// Run emits only versioned JSON on stdout; terminal bytes use bridge-attach.
func Run(ctx context.Context, in io.Reader, out io.Writer, backend tmux.BridgeBackend, lang i18n.Lang, interval time.Duration) error {
	if interval < 250*time.Millisecond || interval > 10*time.Second {
		return errors.New("INVALID_REQUEST")
	}
	server, err := New(backend, lang)
	if err != nil {
		return err
	}
	defer server.Close()
	encoder := json.NewEncoder(out)
	publish := func() error {
		snapshot, _, err := server.observe(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Version  int      `json:"version"`
			Type     string   `json:"type"`
			Snapshot Snapshot `json:"snapshot"`
		}{Version, "snapshot", snapshot})
	}
	if err := publish(); err != nil {
		return err
	}
	lines := make(chan []byte, 1)
	readErrors := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4096), MaxRequestBytes+1)
		defer close(lines)
		for scanner.Scan() {
			raw := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- raw:
			case <-ctx.Done():
				return
			}
		}
		readErrors <- scanner.Err()
	}()
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case raw, ok := <-lines:
			if !ok {
				return <-readErrors
			}
			response := server.Handle(ctx, raw)
			if !idPattern.MatchString(response.RequestID) {
				return fmt.Errorf("INVALID_REQUEST")
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
		case <-timer.C:
			if err := publish(); err != nil {
				return err
			}
		}
	}
}

// PublicErrorCode never includes native stderr, arguments or metadata.
func PublicErrorCode(err error) string { return errorCode(err) }
