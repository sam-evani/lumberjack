package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Home        key.Binding
	End         key.Binding
	Select      key.Binding
	SelectAll   key.Binding
	Clear       key.Binding
	Filter      key.Binding
	Sort        key.Binding
	Delete      key.Binding
	ForceDelete key.Binding
	Quit        key.Binding
	Cancel      key.Binding
	Confirm     key.Binding
}

var keys = keyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k")),
	Down:        key.NewBinding(key.WithKeys("down", "j")),
	PageUp:      key.NewBinding(key.WithKeys("pgup", "ctrl+u")),
	PageDown:    key.NewBinding(key.WithKeys("pgdown", "ctrl+d")),
	Home:        key.NewBinding(key.WithKeys("home", "g")),
	End:         key.NewBinding(key.WithKeys("end", "G")),
	Select:      key.NewBinding(key.WithKeys(" ", "space")),
	SelectAll:   key.NewBinding(key.WithKeys("a")),
	Clear:       key.NewBinding(key.WithKeys("c")),
	Filter:      key.NewBinding(key.WithKeys("/", "f")),
	Sort:        key.NewBinding(key.WithKeys("s")),
	Delete:      key.NewBinding(key.WithKeys("d")),
	ForceDelete: key.NewBinding(key.WithKeys("D")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Cancel:      key.NewBinding(key.WithKeys("esc", "n")),
	Confirm:     key.NewBinding(key.WithKeys("enter", "y")),
}

func keyMatches(msg tea.KeyMsg, binding key.Binding) bool {
	return key.Matches(msg, binding)
}
