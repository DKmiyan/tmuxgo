package tmux

import (
	"fmt"
	"os"
	"sort"
)

// ListSockets discovers the current user's tmux server sockets
// (/tmp/tmux-<uid>/*), excluding the default socket. Stale entries are
// returned as-is; opening one simply yields an empty tree.
func ListSockets() ([]string, error) {
	dir := fmt.Sprintf("/tmp/tmux-%d", os.Getuid())
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Name() == "default" {
			continue
		}
		if e.Type()&os.ModeSocket == 0 {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
