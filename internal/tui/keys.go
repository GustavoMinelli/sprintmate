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
	enqueue     key.Binding
	monitor     key.Binding
	quit        key.Binding
}

func newKeymap(k config.Keys) keymap {
	return keymap{
		launch:      bind(k.Launch),
		switchAgent: bind(k.SwitchAgent),
		refresh:     bind(k.Refresh),
		openBrowser: bind(k.OpenBrowser),
		settings:    bind(k.Settings),
		enqueue:     bind(k.Enqueue),
		monitor:     bind(k.Monitor),
		quit:        bind(k.Quit),
	}
}

func bind(keys []string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}

// monitorKeymap holds the queue monitor's bindings.
type monitorKeymap struct {
	open    key.Binding // open the selected job's review (launch key = enter)
	approve key.Binding
	back    key.Binding
	quit    key.Binding
}

func newMonitorKeymap(k config.Keys) monitorKeymap {
	return monitorKeymap{
		open:    bind(k.Launch),
		approve: bind(k.Approve),
		back:    bind(k.Back),
		quit:    bind(k.Quit),
	}
}

// reviewKeymap holds the review screen's bindings. Quit is intercepted by the
// monitor before keys reach the review sub-model, so it isn't listed here.
type reviewKeymap struct {
	tab     key.Binding
	approve key.Binding
	ship    key.Binding
	back    key.Binding
}

func newReviewKeymap(k config.Keys) reviewKeymap {
	return reviewKeymap{
		tab:     bind(k.Tab),
		approve: bind(k.Approve),
		ship:    bind(k.Ship),
		back:    bind(k.Back),
	}
}
