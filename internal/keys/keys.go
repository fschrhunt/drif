// Package keys names the things drift can do and binds them to keystrokes.
// Every action is rebindable; the only rule is that one key does one thing.
package keys

import (
	"errors"
	"fmt"
	"strings"
)

// Actions. These names are the vocabulary of the [keys] section, so they are
// part of the public interface and should not be renamed casually.
const (
	Quit          = "quit"
	Down          = "down"
	Up            = "up"
	HalfDown      = "half_down"
	HalfUp        = "half_up"
	PageDown      = "page_down"
	PageUp        = "page_up"
	Top           = "top"
	Bottom        = "bottom"
	NextFile      = "next_file"
	PrevFile      = "prev_file"
	NextHunk      = "next_hunk"
	PrevHunk      = "prev_hunk"
	CycleLayout   = "cycle_layout"
	ToggleNumbers = "toggle_numbers"
	Search        = "search"
	NextMatch     = "next_match"
	PrevMatch     = "prev_match"
	Help          = "help"
	Command       = "command"
	Reload        = "reload"
)

// All lists every action in a stable order, for help text and validation.
var All = []string{
	Quit, Down, Up, HalfDown, HalfUp, PageDown, PageUp, Top, Bottom,
	NextFile, PrevFile, NextHunk, PrevHunk, CycleLayout, ToggleNumbers,
	Search, NextMatch, PrevMatch, Help, Command, Reload,
}

// Defaults is the shipped keymap. Several actions accept more than one key,
// which is why bindings are slices.
func Defaults() map[string][]string {
	return map[string][]string{
		Quit:          {"q", "ctrl+c"},
		Down:          {"j", "down"},
		Up:            {"k", "up"},
		HalfDown:      {"ctrl+d"},
		HalfUp:        {"ctrl+u"},
		PageDown:      {" ", "f", "pgdown"},
		PageUp:        {"b", "pgup"},
		Top:           {"g", "home"},
		Bottom:        {"G", "end"},
		NextFile:      {"tab", "]"},
		PrevFile:      {"shift+tab", "["},
		NextHunk:      {"}"},
		PrevHunk:      {"{"},
		CycleLayout:   {"s"},
		ToggleNumbers: {"t"},
		Search:        {"/"},
		NextMatch:     {"n"},
		PrevMatch:     {"N"},
		Help:          {"?"},
		Command:       {":"},
		Reload:        {"R"},
	}
}

// Map is a resolved keymap. Key answers "what does this press do"; Action
// answers "what presses trigger this", in the order they were written, which is
// what help and `drift config` print.
type Map struct {
	Key    map[string]string
	Action map[string][]string
}

// Build merges user overrides onto the defaults. Replacing an action's list
// replaces all of that action's keys, which is what someone writing
// `down = ["n"]` expects.
func Build(overrides map[string][]string) (*Map, error) {
	merged := Defaults()
	for action, list := range overrides {
		if !known(action) {
			return nil, fmt.Errorf("unknown action %q; known actions: %s", action, strings.Join(All, ", "))
		}
		merged[action] = list
	}

	m := &Map{Key: make(map[string]string, len(merged)), Action: make(map[string][]string, len(All))}
	for _, action := range All {
		for _, raw := range merged[action] {
			key, err := Normalize(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", action, err)
			}
			if other, taken := m.Key[key]; taken && other != action {
				return nil, fmt.Errorf("key %q is bound to both %q and %q", key, other, action)
			}
			m.Key[key] = action
			m.Action[action] = append(m.Action[action], key)
		}
	}
	return m, nil
}

// Display renders a binding the way it is written in a config file, so output
// can be pasted back into one.
func Display(k string) string {
	if k == " " {
		return "space"
	}
	return k
}

// Normalize turns a written binding into the string bubbletea reports for that
// key press, so config and runtime agree about what "space" means.
func Normalize(k string) (string, error) {
	switch k {
	case "":
		return "", errors.New("empty key")
	case "space":
		return " ", nil
	case "escape":
		return "esc", nil
	}
	lower := strings.ToLower(k)
	if strings.HasPrefix(lower, "ctrl+") || strings.HasPrefix(lower, "alt+") {
		mod, rest, _ := strings.Cut(lower, "+")
		if rest == "" {
			return "", fmt.Errorf("%q has no key after the modifier", k)
		}
		// bubbletea reports modified keys in lower case, so "Ctrl+D" and
		// "ctrl+d" must bind the same thing.
		return mod + "+" + rest, nil
	}
	if len([]rune(k)) > 1 {
		// Multi-rune names are bubbletea's own ("pgdown", "shift+tab", ...).
		// Sequences like "gg" are not supported, so say so rather than
		// silently binding only the first key.
		if !isNamed(k) {
			return "", fmt.Errorf("%q is not a key; multi-key sequences are not supported", k)
		}
	}
	return k, nil
}

func isNamed(k string) bool {
	switch k {
	case "up", "down", "left", "right", "tab", "shift+tab", "enter", "esc",
		"backspace", "delete", "home", "end", "pgup", "pgdown", "insert", " ":
		return true
	}
	return false
}

// knownSet is All as a set, because Bind looks up more often than it iterates.
var knownSet = func() map[string]bool {
	m := make(map[string]bool, len(All))
	for _, a := range All {
		m[a] = true
	}
	return m
}()

func known(action string) bool { return knownSet[action] }
