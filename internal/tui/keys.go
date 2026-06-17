package tui

import (
	"charm.land/bubbles/v2/key"

	"github.com/GustavoMinelli/sprintmate/internal/config"
)

// keymap holds the dashboard key bindings, built from the user's config.
type keymap struct {
	up          key.Binding
	down        key.Binding
	launch      key.Binding
	switchAgent key.Binding
	refresh     key.Binding
	openBrowser key.Binding
	search      key.Binding
	settings    key.Binding
	quit        key.Binding
}

func newKeymap(k config.Keys) keymap {
	return keymap{
		up:          bind(k.Up),
		down:        bind(k.Down),
		launch:      bind(k.Launch),
		switchAgent: bind(k.SwitchAgent),
		refresh:     bind(k.Refresh),
		openBrowser: bind(k.OpenBrowser),
		search:      bind(k.Search),
		settings:    bind(k.Settings),
		quit:        bind(k.Quit),
	}
}

func bind(keys []string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}
