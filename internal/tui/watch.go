package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// reloadMsg asks the model to re-read the working tree.
type reloadMsg struct{}

// maxWatchedDirs bounds how much of a large repository is watched.
const maxWatchedDirs = 3000

// watcher turns filesystem activity into a single reload per burst.
type watcher struct {
	fw     *fsnotify.Watcher
	events chan struct{}
}

func newWatcher(root string) (*watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &watcher{fw: fw, events: make(chan struct{}, 1)}
	w.addTree(root)
	go w.loop()
	return w, nil
}

func (w *watcher) addTree(root string) {
	added := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root && skipDir(d.Name()) {
			return filepath.SkipDir
		}
		if added >= maxWatchedDirs {
			return filepath.SkipAll
		}
		if w.fw.Add(path) == nil {
			added++
		}
		return nil
	})
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "target", ".venv", "__pycache__", ".next", "dist", "build":
		return true
	}
	return false
}

func (w *watcher) loop() {
	defer close(w.events)
	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if irrelevant(event.Name) {
				continue
			}
			w.addCreatedDir(event)
			// Editors write in bursts; wait for the noise to settle, then
			// collapse everything queued into one reload.
			time.Sleep(120 * time.Millisecond)
		drain:
			for {
				select {
				case event, ok := <-w.fw.Events:
					if !ok {
						return
					}
					w.addCreatedDir(event)
				default:
					break drain
				}
			}
			select {
			case w.events <- struct{}{}:
			default:
			}
		case _, ok := <-w.fw.Errors:
			if !ok {
				return
			}
		}
	}
}

// addCreatedDir starts watching newly created directory trees.
func (w *watcher) addCreatedDir(event fsnotify.Event) {
	if event.Op&fsnotify.Create == 0 || skipDir(filepath.Base(event.Name)) {
		return
	}
	if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
		w.addTree(event.Name)
	}
}

func irrelevant(name string) bool {
	switch {
	case strings.Contains(name, "/.git/"), strings.HasSuffix(name, "~"):
		return true
	case strings.HasSuffix(name, ".swp"), strings.HasSuffix(name, ".tmp"):
		return true
	}
	return false
}

// wait blocks until the next reload is due.
func (w *watcher) wait() tea.Cmd {
	return func() tea.Msg {
		<-w.events
		return reloadMsg{}
	}
}
