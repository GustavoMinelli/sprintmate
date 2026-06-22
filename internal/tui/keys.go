package tui

import (
	"charm.land/bubbles/v2/key"

	"github.com/GustavoMinelli/sprintmate/internal/config"
)

// keymap holds the dashboard key bindings, built from the user's config.
//
// Navigation (up/down) and filtering (search) are owned by the embedded list's
// own KeyMap (configured in newDashboard), so they are not duplicated here.
type keymap struct {
	launch      key.Binding
	switchAgent key.Binding
	refresh     key.Binding
	openBrowser key.Binding
	settings    key.Binding
	quit        key.Binding
}

func newKeymap(k config.Keys) keymap {
	return keymap{
		launch:      bind(k.Launch),
		switchAgent: bind(k.SwitchAgent),
		refresh:     bind(k.Refresh),
		openBrowser: bind(k.OpenBrowser),
		settings:    bind(k.Settings),
		quit:        bind(k.Quit),
	}
}

func bind(keys []string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}
