package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBridgeGuardRejectsRestartBetweenProbeAndMutation(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	real, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	b := newTestBackend(t)
	id, err := b.NewSessionID("original", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := b.BridgeServer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	shim := filepath.Join(directory, "tmux-proxy")
	marker := filepath.Join(directory, "once")
	code := `#!` + python + `
import os,sys,subprocess
args=sys.argv[1:]
real=os.environ['TMUXGO_TEST_REAL']
if 'if-shell' in args and not os.path.exists(os.environ['TMUXGO_TEST_MARKER']):
 open(os.environ['TMUXGO_TEST_MARKER'],'x').close()
 base=[real,'-u','-L',os.environ['TMUXGO_TEST_SOCKET']]
 subprocess.run(base+['kill-session','-t',os.environ['TMUXGO_TEST_SESSION']],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 subprocess.run(base+['-f','/dev/null','new-session','-d','-s','replacement'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
os.execv(real,[real]+args)
`
	if err := os.WriteFile(shim, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUXGO_TEST_REAL", real)
	t.Setenv("TMUXGO_TEST_MARKER", marker)
	t.Setenv("TMUXGO_TEST_SOCKET", b.socket)
	t.Setenv("TMUXGO_TEST_SESSION", id)
	b.bin = shim
	defer func() { b.bin = real }()
	_, err = b.BridgeMutate(context.Background(), before, []string{"kill-session", "-t", id}, "")
	if !errors.Is(err, ErrBridgeGeneration) {
		t.Fatalf("missing in-server generation guard: %v", err)
	}
	tree, err := b.BridgeTree(context.Background())
	if err != nil || len(tree) != 1 || tree[0].Name != "replacement" {
		t.Fatalf("replacement server was mutated: %v %+v", err, tree)
	}
}
