package panels

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ServiceChangedMsg is broadcast by the AppModel to every panel when the
// active service changes. Panels filter their store reads to this name.
type ServiceChangedMsg struct{ Service string }

// Panel is the contract every TUI tab implements.
type Panel interface {
	tea.Model
	SetSize(width, height int)
	Title() string
}

// Placeholder is a stub panel used by the Phase 4 shell. Real panels will
// replace these one phase at a time.
type Placeholder struct {
	title         string
	width, height int
}

func NewPlaceholder(title string) *Placeholder { return &Placeholder{title: title} }

func (p *Placeholder) Title() string                { return p.title }
func (p *Placeholder) SetSize(w, h int)             { p.width, p.height = w, h }
func (p *Placeholder) Init() tea.Cmd                { return nil }
func (p *Placeholder) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return p, nil }

func (p *Placeholder) View() string {
	return p.title + " — coming soon"
}
