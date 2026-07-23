package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// InsideTmux reports whether the process runs inside a tmux client.
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// AttachCmd builds the command that takes the user to target:
// switch-client inside tmux, attach-session outside. Stdio is left for the
// caller (bubbletea's ExecProcess, or the CLI wiring it to the terminal).
func (t *Tmux) AttachCmd(target string) *exec.Cmd {
	if InsideTmux() {
		return t.command("switch-client", "-t", target)
	}
	return t.command("attach-session", "-t", target)
}

// Attach runs AttachCmd on the current terminal and waits for it.
func (t *Tmux) Attach(target string) error {
	cmd := t.AttachCmd(target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("attach %s: %w", target, err)
	}
	return nil
}

// SwitchToLast switches the current client to the previously active session.
func (t *Tmux) SwitchToLast() error {
	return t.run("switch-client", "-l")
}

// NewSessionID creates a detached session and returns its tmux ID.
func (t *Tmux) NewSessionID(name string) (string, error) {
	args := []string{"new-session", "-d", "-P", "-F", "#{session_id}"}
	if name != "" {
		args = append(args, "-s", name)
	}
	out, err := t.output(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// HumanAge renders a duration since t compactly ("12s", "3m", "2h", "5d").
func HumanAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
